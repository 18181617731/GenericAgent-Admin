import { useEffect, useState } from 'react'
import { api } from '../lib/api'
import {
  UPDATE_NOTIFICATION_EVENT, UPDATE_NOTIFICATION_KEY, UPDATE_CHECK_RETRY_MS, UPDATE_CHECK_TIMEOUT_MS,
  notificationRelease, readNotificationPreferences, saveNotificationPreferences,
  readUpdateCheckCache, saveUpdateCheckCache, updateCheckCacheRemaining,
} from '../lib/updateNotifications'

export function useUpdateNotificationPreferences() {
  const [preferences, setPreferences] = useState(readNotificationPreferences)
  useEffect(() => {
    const sync = event => {
      if (event.type !== 'storage' || event.key === UPDATE_NOTIFICATION_KEY || event.key === null) {
        setPreferences(readNotificationPreferences())
      }
    }
    window.addEventListener(UPDATE_NOTIFICATION_EVENT, sync)
    window.addEventListener('storage', sync)
    return () => {
      window.removeEventListener(UPDATE_NOTIFICATION_EVENT, sync)
      window.removeEventListener('storage', sync)
    }
  }, [])
  return { preferences, setEnabled: enabled => setPreferences(saveNotificationPreferences({ enabled })) }
}

// One host at the application root; version panels only subscribe to preferences.
// Automatic checks are read-only, cached across reloads, and quiet on failure.
export function useVersionNotification() {
  const { preferences, setEnabled } = useUpdateNotificationPreferences()
  const [snapshot, setSnapshot] = useState(null)
  useEffect(() => {
    if (!preferences.enabled) return undefined
    let disposed = false
    let controller = null
    let timer
    const tick = async () => {
      if (disposed || controller || document.visibilityState === 'hidden' || navigator.onLine === false) return
      clearTimeout(timer)
      controller = new AbortController()
      const requestController = controller
      const requestTimeout = setTimeout(() => requestController.abort(), UPDATE_CHECK_TIMEOUT_MS)
      let currentVersion
      let delay = UPDATE_CHECK_RETRY_MS
      try {
        const info = await api('/api/version/info', { signal: controller.signal })
        if (disposed) return
        currentVersion = info?.version
        if (typeof currentVersion !== 'string' || !currentVersion) throw new Error('Missing current version')
        const cached = readUpdateCheckCache()
        const remaining = updateCheckCacheRemaining(cached, currentVersion)
        if (remaining) {
          setSnapshot({ currentVersion, check: cached.check })
          delay = remaining
        } else {
          const check = await api('/api/version/check', { signal: controller.signal })
          if (disposed) return
          if (typeof check?.update !== 'boolean') throw new Error('Invalid version check')
          const saved = saveUpdateCheckCache(currentVersion, check)
          setSnapshot({ currentVersion, check })
          delay = updateCheckCacheRemaining(saved, currentVersion)
        }
      } catch {
        if (disposed) return
        if (currentVersion) saveUpdateCheckCache(currentVersion, null)
        setSnapshot(null)
      } finally {
        clearTimeout(requestTimeout)
        controller = null
        if (!disposed) timer = setTimeout(tick, Math.max(1000, delay))
      }
    }
    const wake = () => { void tick() }
    wake()
    document.addEventListener('visibilitychange', wake)
    window.addEventListener('online', wake)
    return () => {
      disposed = true
      clearTimeout(timer)
      controller?.abort()
      document.removeEventListener('visibilitychange', wake)
      window.removeEventListener('online', wake)
    }
  }, [preferences.enabled])
  const release = notificationRelease(snapshot?.check, preferences, snapshot?.currentVersion)
  return {
    release, check: snapshot?.check, setEnabled,
    dismiss: () => saveNotificationPreferences({ dismissedVersion: release }),
  }
}

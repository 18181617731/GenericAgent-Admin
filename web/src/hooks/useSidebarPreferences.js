import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { normalizeSidebarPreferences, readSidebarPreferences, sidebarPreferencesStorageKey } from '../lib/chatSidebarPreferences'

const endpoint = '/api/chat/sidebar-preferences'
const applyEdit = (preferences, key, value) => normalizeSidebarPreferences({
  ...preferences, [key]: typeof value === 'function' ? value(preferences[key]) : value,
})

const readLegacy = () => {
  try {
    const value = JSON.parse(localStorage.getItem(sidebarPreferencesStorageKey) || 'null')
    return value && typeof value === 'object' && !Array.isArray(value) && Object.keys(value).length ? normalizeSidebarPreferences(value) : null
  } catch { return null }
}

export function useSidebarPreferences() {
  const [legacy] = useState(readLegacy)
  const [preferences, setPreferences] = useState(readSidebarPreferences)
  const [error, setError] = useState('')
  const state = useRef({ preferences, ready: false, edits: [], pending: {}, legacy })
  const pumpRef = useRef(null)
  const publish = useCallback(next => {
    state.current.preferences = next
    setPreferences(next)
    try { localStorage.setItem(sidebarPreferencesStorageKey, JSON.stringify(next)) } catch { /* Optional local cache. */ }
  }, [])

  useEffect(() => {
    let alive = true
    let loading = false
    let saving = false
    const current = state.current
    current.ready = false
    const save = async () => {
      if (!alive || !current.ready || saving) return
      saving = true
      try {
        while (alive && Object.keys(current.pending).length) {
          const patch = current.pending
          current.pending = {}
          try {
            const response = await api(endpoint, { method: 'PUT', body: JSON.stringify({ preferences: patch }) })
            if (!alive) { current.pending = { ...patch, ...current.pending }; return }
            publish(normalizeSidebarPreferences({ ...response.preferences, ...current.pending }))
            setError('')
          } catch (err) {
            current.pending = { ...patch, ...current.pending }
            if (alive) setError(err.message || String(err))
            break
          }
        }
      } finally { saving = false }
    }
    const load = async (refresh = false) => {
      if (!alive || loading) return
      if (current.ready && (!refresh || saving || Object.keys(current.pending).length)) { void save(); return }
      current.ready = false
      loading = true
      try {
        let response = await api(endpoint)
        if (!alive) return
        if (response.preferences == null && current.legacy) {
          // Initialize is atomic: a different browser's saved settings always win.
          response = await api(endpoint, { method: 'PUT', body: JSON.stringify({ preferences: current.legacy, initialize: true }) })
          if (!alive) return
        }
        let next = normalizeSidebarPreferences({ ...response.preferences, ...current.pending })
        for (const [key, value] of current.edits) next = applyEdit(next, key, value)
        for (const [key] of current.edits) current.pending[key] = next[key]
        current.edits = []
        current.ready = true
        publish(next)
        setError('')
        void save()
      } catch (err) {
        if (alive) setError(err.message || String(err))
      } finally { loading = false }
    }
    pumpRef.current = load
    void load()
    const refresh = () => { void load(true) }
    window.addEventListener('online', refresh)
    window.addEventListener('focus', refresh)
    return () => {
      alive = false
      pumpRef.current = null
      window.removeEventListener('online', refresh)
      window.removeEventListener('focus', refresh)
    }
  }, [publish])

  const update = useCallback((key, value) => {
    const current = state.current
    const next = applyEdit(current.preferences, key, value)
    if (current.ready) current.pending[key] = next[key]
    else current.edits.push([key, value])
    publish(next)
    void pumpRef.current?.()
  }, [publish])
  return { preferences, update, error }
}

import { useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import {
  VERSION_RELOAD_DELAY_MS,
  VERSION_RELOAD_RETRY_MS,
  beginVersionRestartGrace,
  shouldReloadAfterVersionUpdate,
  shouldReportVersionPollError,
  versionMatchesExpectedRelease,
} from '../lib/versionUpdatePolling'

// GA Admin releases, GA source (git) updates, and the login autostart entry.
// These three all answer "is this install current and how does it come up".
export function useVersionUpdates({ t, lang, setMsg, setBusy, reload }) {
  const [info, setInfo] = useState(null)
  const [check, setCheck] = useState(null)
  const [status, setStatus] = useState(null)
  const [busy, setVersionBusy] = useState(false)
  const [gitBusy, setGitBusy] = useState(false)
  const [gitResult, setGitResult] = useState(null)
  const [gitStatus, setGitStatus] = useState(null)
  const [autostart, setAutostart] = useState(null)
  const restartGraceUntil = useRef(0)
  const needsReload = useRef(false)
  const observedRunning = useRef(false)

  const refreshStatus = async () => {
    const d = await api('/api/version/status')
    setStatus(d)
    if (d?.check) setCheck(d.check)
    if (d?.running) observedRunning.current = true
    return d
  }

  // Loaded alongside the workspace boot sequence so the shell can render the
  // version card without a second round trip.
  const loadSnapshot = async () => {
    const [auto, ver, stat] = await Promise.all([
      api('/api/autostart/status').catch(e => ({ supported:false, enabled:false, error:e.message })),
      api('/api/version/info').catch(e => ({ error:e.message })),
      api('/api/version/status').catch(() => null),
    ])
    setAutostart(auto)
    setInfo(ver)
    if (stat?.id || stat?.stage) setStatus(stat)
    return { autostart: auto, info: ver, status: stat }
  }

  useEffect(() => {
    let stopped = false
    const tick = async () => {
      try {
        const d = await refreshStatus()
        if (needsReload.current && shouldReloadAfterVersionUpdate(d, observedRunning.current)) {
          if (versionMatchesExpectedRelease(info?.version, check?.latest?.tag_name || '')) {
            setTimeout(() => window.location.reload(), VERSION_RELOAD_DELAY_MS)
            return
          }
        }
        if (!stopped && d?.running) setTimeout(tick, VERSION_RELOAD_RETRY_MS)
      } catch (err) {
        if (shouldReportVersionPollError(restartGraceUntil.current)) console.error('Version status poll error:', err)
        if (!stopped) setTimeout(tick, VERSION_RELOAD_RETRY_MS)
      }
    }
    tick()
    return () => { stopped = true }
  }, [info, check])

  useEffect(() => {
    if (!status?.running) return undefined
    const timer = setInterval(() => refreshStatus().catch(e => {
      if (shouldReportVersionPollError(restartGraceUntil.current)) setMsg(e.message)
    }), VERSION_RELOAD_RETRY_MS)
    return () => clearInterval(timer)
  }, [status?.running])

  const checkVersion = async () => {
    setVersionBusy(true)
    try {
      const d = await api('/api/version/check')
      setCheck(d)
      setMsg(d.update ? t.overview.versionFound(d.latest?.tag_name || '') : t.overview.versionCurrent)
    } catch (e) { setMsg(e.message) } finally { setVersionBusy(false) }
  }

  const updateVersion = async () => {
    if (!confirmDanger('version-update', t.overview.versionUpdateConfirm)) return
    setVersionBusy(true)
    try {
      restartGraceUntil.current = beginVersionRestartGrace()
      needsReload.current = true
      const d = await api('/api/version/update', { dangerous:true, method:'POST', body:'{}' })
      setStatus(d)
      setMsg(t.overview.updateQueued)
    } catch (e) {
      restartGraceUntil.current = 0
      needsReload.current = false
      setMsg(e.message)
    } finally { setVersionBusy(false) }
  }

  const checkSource = async () => {
    setGitBusy(true); setMsg('')
    try {
      const d = await api('/api/ga/git-status?remote=1')
      setGitStatus(d)
      setMsg(d.upstream_configured === false
        ? t.overview.sourceMissingMessage
        : (d.latest ? t.overview.sourceCurrentMessage : t.overview.sourceBehindMessage(d.behind || 0)))
    } catch (e) { setGitStatus({ ok:false, error:e.message }); setMsg(e.message) } finally { setGitBusy(false) }
  }

  const updateSource = async () => {
    if (!confirmDanger('ga-git-update', t.overview.sourceCheckConfirm)) return
    setGitBusy(true); setMsg('')
    try {
      const d = await api('/api/ga/git-update', { dangerous:true, method:'POST', body: '{}' })
      setGitResult(d)
      setMsg(d.changed ? t.overview.sourceUpdatedMessage(d.before, d.after) : t.overview.sourceCurrentMessage)
      setGitStatus(await api('/api/ga/git-status'))
      await reload?.()
    } catch (e) { setMsg(e.message) } finally { setGitBusy(false) }
  }

  const toggleAutostart = async () => {
    const next = !autostart?.enabled
    const prompt = lang === 'zh'
      ? (next ? '启用 GA Admin 开机自启动？' : '禁用 GA Admin 开机自启动？')
      : (next ? 'Enable GA Admin at login?' : 'Disable GA Admin at login?')
    if (!confirmDanger('admin-autostart', prompt)) return
    setBusy(true); setMsg('')
    try {
      const d = await api(next ? '/api/autostart/enable' : '/api/autostart/disable', { dangerous:true, method:'POST' })
      setAutostart(d)
      setMsg(t.hints.autostartChanged)
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  return {
    info, check, status, busy, gitBusy, gitResult, gitStatus, autostart,
    loadSnapshot, refreshStatus, checkVersion, updateVersion, checkSource, updateSource, toggleAutostart,
  }
}

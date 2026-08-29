import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import { readLocalCmdStream, waitForLocalCmdReconnect } from './localCmdStream'
import {
  appendTerminalChunk,
  clearTerminalBuffer,
  createTerminalBuffer,
  encodeTerminalBase64,
  encodeTerminalInput,
  terminalShortcutBytes,
} from './localCmdTerminal'

export const LOCAL_CMD_SESSION_STORAGE_KEY = 'ga-admin.remote-cmd.session'
const INITIAL_TERMINAL_SIZE = { cols: 120, rows: 32 }
const LOCAL_CMD_RESIZE_DELAY_MS = 120

const terminalBytes = value => {
  if (ArrayBuffer.isView(value)) return new Uint8Array(value.buffer, value.byteOffset, value.byteLength).slice()
  if (value instanceof ArrayBuffer) return new Uint8Array(value).slice()
  return encodeTerminalInput(value)
}

export const fetchLocalCmdDirectories = path => {
  const query = String(path || '').trim()
  const suffix = query ? `?path=${encodeURIComponent(query)}` : ''
  return api(`/api/local-cmd/directories${suffix}`)
}

export const createLocalCmdSession = (path, cols, rows) => api('/api/local-cmd/sessions', {
  dangerous: true, method: 'POST', body: JSON.stringify({ path, cols, rows }),
})

export const sendLocalCmdInput = (id, bytes) => api(`/api/local-cmd/sessions/${encodeURIComponent(id)}/input`, {
  dangerous: true, method: 'POST', body: JSON.stringify({ base64: encodeTerminalBase64(bytes) }),
})

export const resizeLocalCmdSession = (id, cols, rows) => api(`/api/local-cmd/sessions/${encodeURIComponent(id)}/resize`, {
  dangerous: true, method: 'POST', body: JSON.stringify({ cols, rows }),
})

export const deleteLocalCmdSession = id => api(`/api/local-cmd/sessions/${encodeURIComponent(id)}`, {
  dangerous: true, method: 'DELETE',
})

const readSavedSessionID = () => {
  try { return window.localStorage.getItem(LOCAL_CMD_SESSION_STORAGE_KEY) || '' } catch { return '' }
}

const saveSessionID = id => {
  try { window.localStorage.setItem(LOCAL_CMD_SESSION_STORAGE_KEY, id) } catch { /* best effort */ }
}

const clearSavedSessionID = () => {
  try { window.localStorage.removeItem(LOCAL_CMD_SESSION_STORAGE_KEY) } catch { /* best effort */ }
}

const useLocalCmdState = () => {
  const [path, setPath] = useState('')
  const [directories, setDirectories] = useState({ current: '', parent: '', roots: [], entries: [] })
  const [directoryBusy, setDirectoryBusy] = useState(false)
  const [session, setSession] = useState(null)
  const [notice, setNotice] = useState(null)
  const [connection, setConnection] = useState('idle')
  const [busy, setBusy] = useState('')
  const [size, setSize] = useState(INITIAL_TERMINAL_SIZE)
  const [terminalRevision, setTerminalRevision] = useState(0)
  const [terminalClearToken, setTerminalClearToken] = useState(0)
  const streamSeq = useRef(0)
  const sessionStatus = useRef('')
  const terminalBuffer = useRef(createTerminalBuffer())
  const inputTail = useRef(Promise.resolve())
  const inputGeneration = useRef(0)
  const pendingResize = useRef(null)
  const resizeTimer = useRef(0)
  const lastResize = useRef(INITIAL_TERMINAL_SIZE)
  const state = {
    path, setPath, directories, setDirectories, directoryBusy, setDirectoryBusy,
    session, setSession, notice, setNotice, connection, setConnection, busy, setBusy,
    size, setSize, streamSeq, sessionStatus, terminalBuffer, terminalRevision,
    setTerminalRevision, terminalClearToken, setTerminalClearToken, inputTail,
    inputGeneration, pendingResize, resizeTimer, lastResize,
  }
  const stateRef = useRef(state)
  stateRef.current = state
  return { ...state, stateRef, terminalChunks: terminalBuffer.current.chunks }
}

const useDirectoryActions = (text, state) => {
  const { stateRef } = state
  const loadDirectories = useCallback(async (target = '') => {
    stateRef.current.setDirectoryBusy(true)
    try {
      const result = await fetchLocalCmdDirectories(target)
      stateRef.current.setDirectories({ current: result.current || '', parent: result.parent || '', roots: result.roots || [], entries: result.entries || [] })
    } catch (error) {
      stateRef.current.setNotice({ kind: 'error', message: error.message || text.directoryError })
    } finally { stateRef.current.setDirectoryBusy(false) }
  }, [stateRef, text])
  const chooseDirectory = useCallback(target => {
    stateRef.current.setPath(target)
    loadDirectories(target)
  }, [loadDirectories, stateRef])
  return { loadDirectories, chooseDirectory }
}

const resetTerminal = state => {
  state.streamSeq.current = 0
  state.sessionStatus.current = ''
  state.inputGeneration.current += 1
  clearTerminalBuffer(state.terminalBuffer.current)
  state.pendingResize.current = null
  window.clearTimeout(state.resizeTimer.current)
  state.resizeTimer.current = 0
  state.lastResize.current = INITIAL_TERMINAL_SIZE
  state.setTerminalClearToken(token => token + 1)
  state.setTerminalRevision(revision => revision + 1)
}

const clearTerminal = state => {
  clearTerminalBuffer(state.terminalBuffer.current)
  state.setTerminalClearToken(token => token + 1)
  state.setTerminalRevision(revision => revision + 1)
}

const useSessionActions = (text, state) => {
  const { stateRef } = state
  const create = useCallback(async () => {
    const current = stateRef.current
    const selected = current.path.trim()
    if (!selected) { current.setNotice({ kind: 'error', message: text.required }); return }
    if (!confirmDanger('local-cmd-session-create', text.confirm(selected))) {
      current.setNotice({ kind: 'success', message: text.createCancelled }); return
    }
    current.setBusy('create')
    current.setNotice({ kind: 'pending', message: text.creating })
    try {
      const result = await createLocalCmdSession(selected, current.size.cols, current.size.rows)
      saveSessionID(result.id)
      resetTerminal(current)
      current.sessionStatus.current = result.status || 'running'
      current.lastResize.current = current.size
      current.setSession(result)
      current.setConnection('connecting')
      current.setNotice({ kind: 'success', message: text.created })
    } catch (error) { current.setNotice({ kind: 'error', message: error.message })
    } finally { stateRef.current.setBusy('') }
  }, [stateRef, text])

  const end = useCallback(async () => {
    const current = stateRef.current
    if (!current.session?.id || !confirmDanger('local-cmd-session-delete', text.endConfirm)) {
      if (current.session?.id) current.setNotice({ kind: 'success', message: text.endCancelled })
      return
    }
    current.setBusy('end')
    try {
      await deleteLocalCmdSession(current.session.id)
      clearSavedSessionID()
      resetTerminal(current)
      current.setSession(null)
      current.setConnection('idle')
      current.setNotice({ kind: 'success', message: text.ended })
    } catch (error) { current.setNotice({ kind: 'error', message: error.message })
    } finally { stateRef.current.setBusy('') }
  }, [stateRef, text])
  return { create, end }
}

const useInputActions = (text, state) => {
  const { stateRef } = state
  const sendBytes = useCallback(bytes => {
    const current = stateRef.current
    const sessionID = current.session?.id
    const generation = current.inputGeneration.current
    const payload = terminalBytes(bytes)
    if (!sessionID || !payload.length || current.sessionStatus.current !== 'running') return Promise.resolve()
    const send = current.inputTail.current.catch(() => {}).then(async () => {
      const latest = stateRef.current
      if (latest.inputGeneration.current !== generation || latest.session?.id !== sessionID || latest.sessionStatus.current !== 'running') return
      try { await sendLocalCmdInput(sessionID, payload) } catch (error) { latest.setNotice({ kind: 'error', message: error.message || text.inputError }) }
    })
    current.inputTail.current = send
    return send
  }, [stateRef, text])
  const sendText = useCallback(value => sendBytes(encodeTerminalInput(value)), [sendBytes])
  const sendShortcut = useCallback(key => sendBytes(terminalShortcutBytes(key)), [sendBytes])
  return { sendBytes, sendText, sendShortcut }
}

const useResizeActions = state => {
  const { stateRef } = state
  const flushResize = useCallback(async () => {
    const current = stateRef.current
    current.resizeTimer.current = 0
    const next = current.pendingResize.current
    current.pendingResize.current = null
    if (!next || !current.session?.id || current.sessionStatus.current !== 'running') return
    if (current.lastResize.current.cols === next.cols && current.lastResize.current.rows === next.rows) return
    current.lastResize.current = next
    try { await resizeLocalCmdSession(current.session.id, next.cols, next.rows) } catch (error) {
      current.setNotice({ kind: 'error', message: error.message })
    }
  }, [stateRef])
  const syncTerminalSize = useCallback((cols, rows) => {
    const current = stateRef.current
    const next = { cols: Math.max(1, Math.min(500, Math.round(cols))), rows: Math.max(1, Math.min(200, Math.round(rows))) }
    if (!current.session?.id || current.sessionStatus.current !== 'running') return
    if (current.lastResize.current.cols === next.cols && current.lastResize.current.rows === next.rows && !current.pendingResize.current) return
    current.setSize(size => size.cols === next.cols && size.rows === next.rows ? size : next)
    current.pendingResize.current = next
    window.clearTimeout(current.resizeTimer.current)
    current.resizeTimer.current = window.setTimeout(() => { void flushResize() }, LOCAL_CMD_RESIZE_DELAY_MS)
  }, [flushResize, stateRef])
  return { syncTerminalSize }
}

export const handleLocalCmdEvent = (event, state) => {
  if (event.type !== 'sync' && Number(event.seq) > state.streamSeq.current) state.streamSeq.current = Number(event.seq)
  if (event.type === 'sync') {
    state.sessionStatus.current = event.status || state.sessionStatus.current
    state.setSession(session => session ? { ...session, ...event } : session)
  }
  if (event.type === 'data' && event.bytes?.length) {
    appendTerminalChunk(state.terminalBuffer.current, event.bytes)
    state.setTerminalRevision(revision => revision + 1)
  }
  if (event.type === 'exit') {
    state.sessionStatus.current = 'exited'
    state.setSession(session => session ? { ...session, status: 'exited', exit_code: event.exit_code } : session)
    state.setConnection('exited')
  }
  if (event.type === 'error') state.setNotice({ kind: 'error', message: event.message || 'The remote CMD session failed.' })
}

const useLocalCmdStream = (state, onEvent) => {
  const { stateRef } = state
  const sessionID = state.session?.id
  useEffect(() => {
    if (!sessionID) return undefined
    let stopped = false
    const controller = new AbortController()
    const connect = async () => {
      while (!stopped) {
        stateRef.current.setConnection('connecting')
        try {
          await readLocalCmdStream(sessionID, stateRef.current.streamSeq.current, onEvent, controller.signal)
          if (stopped || stateRef.current.sessionStatus.current === 'exited') return
        } catch (error) {
          if (stopped || error?.name === 'AbortError') return
          stateRef.current.setNotice({ kind: 'error', message: error.message })
        }
        if (stopped) return
        stateRef.current.setConnection('reconnecting')
        try { await waitForLocalCmdReconnect(controller.signal) } catch { return }
      }
    }
    connect()
    return () => { stopped = true; controller.abort() }
  }, [onEvent, sessionID, stateRef])
}

const useLocalCmdBootstrap = (state, loadDirectories) => {
  const { stateRef } = state
  useEffect(() => {
    loadDirectories()
    const stored = readSavedSessionID()
    if (!stored) return undefined
    let cancelled = false
    api(`/api/local-cmd/sessions/${encodeURIComponent(stored)}`).then(result => {
      if (cancelled) return
      stateRef.current.sessionStatus.current = result.status || ''
      if (result.status === 'running' || result.status === 'exited') stateRef.current.setSession(result)
      else clearSavedSessionID()
    }).catch(() => { if (!cancelled) clearSavedSessionID() })
    return () => { cancelled = true }
  }, [loadDirectories, stateRef])
}

export const useLocalCmdController = text => {
  const state = useLocalCmdState()
  const { stateRef } = state
  const directories = useDirectoryActions(text, state)
  const session = useSessionActions(text, state)
  const input = useInputActions(text, state)
  const resize = useResizeActions(state)
  const onEvent = useCallback(event => handleLocalCmdEvent(event, stateRef.current), [stateRef])
  useLocalCmdBootstrap(state, directories.loadDirectories)
  useLocalCmdStream(state, onEvent)
  useEffect(() => () => window.clearTimeout(stateRef.current.resizeTimer.current), [stateRef])
  return {
    ...state, ...directories, ...session, ...input, ...resize,
    clearTerminal: () => clearTerminal(stateRef.current),
    dismiss: () => state.setNotice(null),
  }
}

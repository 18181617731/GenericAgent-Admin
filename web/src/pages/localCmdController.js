import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import { readLocalCmdStream, waitForLocalCmdReconnect } from './localCmdStream'
import { appendTerminalText, encodeTerminalBase64, terminalShortcutBytes } from './localCmdTerminal'

export const LOCAL_CMD_SESSION_STORAGE_KEY = 'ga-admin.remote-cmd.session'

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
  const [output, setOutput] = useState('')
  const [input, setInput] = useState('')
  const [notice, setNotice] = useState(null)
  const [connection, setConnection] = useState('idle')
  const [busy, setBusy] = useState('')
  const [history, setHistory] = useState([])
  const [historyIndex, setHistoryIndex] = useState(-1)
  const [size, setSize] = useState({ cols: 120, rows: 32 })
  const streamSeq = useRef(0)
  const decoder = useRef(new TextDecoder())
  const terminalState = useRef({ pendingCR: false })
  const sessionStatus = useRef('')
  const state = { path, setPath, directories, setDirectories, directoryBusy, setDirectoryBusy, session, setSession, output, setOutput, input, setInput, notice, setNotice, connection, setConnection, busy, setBusy, history, setHistory, historyIndex, setHistoryIndex, size, setSize, streamSeq, decoder, terminalState, sessionStatus }
  const stateRef = useRef(state)
  stateRef.current = state
  return { ...state, stateRef }
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
  state.decoder.current = new TextDecoder()
  state.terminalState.current.pendingCR = false
  state.setOutput('')
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
      current.sessionStatus.current = result.status || 'running'
      resetTerminal(current)
      current.setSession(result)
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
    try { await deleteLocalCmdSession(current.session.id); clearSavedSessionID(); current.sessionStatus.current = ''; current.setSession(null); current.setConnection('idle'); resetTerminal(current); current.setNotice({ kind: 'success', message: text.ended })
    } catch (error) { current.setNotice({ kind: 'error', message: error.message })
    } finally { stateRef.current.setBusy('') }
  }, [stateRef, text])
  return { create, end }
}

const useInputActions = (text, state) => {
  const { stateRef } = state
  const sendBytes = useCallback(async bytes => {
    const current = stateRef.current
    if (!current.session?.id || !bytes?.length || current.sessionStatus.current !== 'running') return
    try { await sendLocalCmdInput(current.session.id, bytes) } catch (error) { current.setNotice({ kind: 'error', message: error.message || text.inputError }) }
  }, [stateRef, text])
  const sendText = useCallback(value => sendBytes(new TextEncoder().encode(value)), [sendBytes])
  const sendCommand = useCallback(() => {
    const current = stateRef.current
    const value = current.input
    if (!value.trim()) { sendText('\r'); return }
    current.setHistory(items => [...items.filter(item => item !== value), value].slice(-50))
    current.setHistoryIndex(-1)
    current.setInput('')
    sendText(`${value}\r`)
  }, [sendText, stateRef])
  const shortcut = useCallback(key => {
    const bytes = terminalShortcutBytes(key)
    if (bytes) sendText(bytes)
  }, [sendText])
  const moveHistory = useCallback(direction => {
    const current = stateRef.current
    current.setHistoryIndex(index => {
      const next = Math.max(-1, Math.min(current.history.length - 1, index + direction))
      current.setInput(next < 0 ? '' : current.history[current.history.length - 1 - next] || '')
      return next
    })
  }, [stateRef])
  return { sendBytes, sendCommand, shortcut, moveHistory }
}

const useResizeAction = (text, state) => {
  const { stateRef } = state
  return useCallback(async () => {
    const current = stateRef.current
    if (!current.session?.id || !confirmDanger('local-cmd-session-resize', text.resizeConfirm)) return
    current.setBusy('resize')
    try { await resizeLocalCmdSession(current.session.id, current.size.cols, current.size.rows); current.setNotice({ kind: 'success', message: text.resized })
    } catch (error) { current.setNotice({ kind: 'error', message: error.message })
    } finally { stateRef.current.setBusy('') }
  }, [stateRef, text])
}

export const handleLocalCmdEvent = (event, state) => {
  if (event.type !== 'sync' && Number(event.seq) > state.streamSeq.current) state.streamSeq.current = Number(event.seq)
  if (event.type === 'sync') {
    state.sessionStatus.current = event.status || state.sessionStatus.current
    state.setSession(session => session ? { ...session, ...event } : session)
  }
  if (event.type === 'data' && event.bytes) {
    const decoded = state.decoder.current.decode(event.bytes, { stream: true })
    if (decoded) state.setOutput(previous => appendTerminalText(previous, decoded, state.terminalState.current))
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
  const resize = useResizeAction(text, state)
  const onEvent = useCallback(event => handleLocalCmdEvent(event, stateRef.current), [stateRef])
  useLocalCmdBootstrap(state, directories.loadDirectories)
  useLocalCmdStream(state, onEvent)
  return { ...state, ...directories, ...session, ...input, resize, dismiss: () => state.setNotice(null) }
}

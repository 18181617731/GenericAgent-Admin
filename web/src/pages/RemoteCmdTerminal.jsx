import React, { useEffect, useRef } from 'react'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import '@xterm/xterm/css/xterm.css'

const TERMINAL_THEME = {
  background: '#151a18',
  foreground: '#ddf7e7',
  cursor: '#f5fbf7',
  cursorAccent: '#151a18',
  selectionBackground: '#28433a',
}

const FIT_DELAY_MS = 80

export function RemoteCmdTerminal({
  chunks = [],
  revision = 0,
  clearToken = 0,
  interactive = true,
  label = 'Remote terminal',
  onData,
  onResize,
}) {
  const hostRef = useRef(null)
  const terminalRef = useRef(null)
  const fitAddonRef = useRef(null)
  const resizeTimerRef = useRef(0)
  const renderedChunkRef = useRef(0)
  const appliedClearTokenRef = useRef(clearToken)
  const lastSizeRef = useRef({ cols: 0, rows: 0 })
  const callbacksRef = useRef({ onData, onResize })
  callbacksRef.current = { onData, onResize }

  useEffect(() => {
    const host = hostRef.current
    if (!host) return undefined
    const terminal = new Terminal({
      allowTransparency: false,
      convertEol: false,
      cursorBlink: true,
      cursorStyle: 'block',
      fontFamily: 'ui-monospace, SFMono-Regular, Consolas, "Liberation Mono", monospace',
      fontSize: 13,
      scrollback: 5000,
      tabStopWidth: 4,
      theme: TERMINAL_THEME,
    })
    const fitAddon = new FitAddon()
    terminal.loadAddon(fitAddon)
    terminal.open(host)
    terminalRef.current = terminal
    fitAddonRef.current = fitAddon

    const emitSize = () => {
      if (!terminalRef.current || !fitAddonRef.current) return
      try { fitAddonRef.current.fit() } catch { return }
      const next = { cols: terminalRef.current.cols, rows: terminalRef.current.rows }
      if (next.cols < 1 || next.rows < 1) return
      if (lastSizeRef.current.cols === next.cols && lastSizeRef.current.rows === next.rows) return
      lastSizeRef.current = next
      callbacksRef.current.onResize?.(next.cols, next.rows)
    }
    const queueFit = () => {
      window.clearTimeout(resizeTimerRef.current)
      resizeTimerRef.current = window.setTimeout(emitSize, FIT_DELAY_MS)
    }
    const resizeObserver = typeof ResizeObserver === 'function' ? new ResizeObserver(queueFit) : null
    resizeObserver?.observe(host)
    window.addEventListener('resize', queueFit)
    const dataSubscription = terminal.onData(data => callbacksRef.current.onData?.(data))
    queueFit()

    return () => {
      window.clearTimeout(resizeTimerRef.current)
      resizeObserver?.disconnect()
      window.removeEventListener('resize', queueFit)
      dataSubscription.dispose()
      terminal.dispose()
      terminalRef.current = null
      fitAddonRef.current = null
      renderedChunkRef.current = 0
      lastSizeRef.current = { cols: 0, rows: 0 }
    }
  }, [])

  useEffect(() => {
    const terminal = terminalRef.current
    if (!terminal) return
    if (chunks.length < renderedChunkRef.current) renderedChunkRef.current = 0
    while (renderedChunkRef.current < chunks.length) {
      terminal.write(chunks[renderedChunkRef.current])
      renderedChunkRef.current += 1
    }
  }, [chunks, revision])

  useEffect(() => {
    const terminal = terminalRef.current
    if (!terminal || appliedClearTokenRef.current === clearToken) return
    appliedClearTokenRef.current = clearToken
    terminal.clear()
    renderedChunkRef.current = 0
  }, [clearToken])

  useEffect(() => {
    if (interactive) terminalRef.current?.focus()
  }, [interactive])

  return <div
    ref={hostRef}
    role="application"
    aria-label={label}
    tabIndex={0}
    className={`local-cmd-terminal-surface${interactive ? ' is-interactive' : ''}`}
    onClick={() => terminalRef.current?.focus()}
    onFocus={() => terminalRef.current?.focus()}
  />
}

export default RemoteCmdTerminal

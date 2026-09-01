import { useEffect, useRef, useState } from 'react'
import { clampTailLines } from '../lib/filesSafety'
import { addInstanceToURL } from '../lib/instanceScope'

// Server-sent log stream for one selected service. The stream is only open while
// the logs page is active; `nonce` lets a retry reopen the same subscription.
export function useLogStream({ active, instanceID = '' }) {
  const [selected, setSelected] = useState('')
  const [lines, setLines] = useState([])
  const [tailLines, setTailLinesRaw] = useState(200)
  const [streamState, setStreamState] = useState('idle')
  const [nonce, setNonce] = useState(0)
  const [follow, setFollow] = useState(true)
  const [filter, setFilter] = useState('')
  const viewRef = useRef(null)

  const setTailLines = (value) => setTailLinesRaw(clampTailLines(value))
  const select = (name) => {
    if (!name) return
    setSelected(name)
    setFollow(true)
    setNonce(value => value + 1)
  }
  const retry = () => setNonce(value => value + 1)
  const clear = () => setLines([])

  useEffect(() => {
    if (!active || !selected) {
      setStreamState('idle')
      return undefined
    }
    const maxLines = clampTailLines(tailLines, 20, 2000)
    const source = new EventSource(addInstanceToURL(`/api/logs/${encodeURIComponent(selected)}/stream?lines=${maxLines}`, instanceID))
    const readPayload = (event) => {
      try { return JSON.parse(event.data) } catch { return null }
    }
    setLines([])
    setStreamState('connecting')
    source.onopen = () => setStreamState('live')
    const replace = (event) => {
      const payload = readPayload(event)
      setLines(Array.isArray(payload?.lines) ? payload.lines.map(String).slice(-maxLines) : [])
    }
    source.addEventListener('snapshot', replace)
    source.addEventListener('reset', replace)
    source.addEventListener('log', (event) => {
      const payload = readPayload(event)
      if (payload?.line === undefined) return
      setLines(current => [...current, String(payload.line)].slice(-maxLines))
    })
    source.onerror = () => setStreamState('reconnecting')
    return () => {
      source.close()
      setStreamState('idle')
    }
  }, [active, selected, tailLines, nonce, instanceID])

  useEffect(() => {
    if (!follow || !active) return undefined
    const frame = window.requestAnimationFrame(() => {
      const view = viewRef.current
      if (view) view.scrollTop = view.scrollHeight
    })
    return () => window.cancelAnimationFrame(frame)
    // `filter` changes which lines are rendered, so following has to re-anchor.
  }, [lines, follow, active, filter])

  const handleScroll = (event) => {
    const view = event.currentTarget
    setFollow(view.scrollHeight - view.scrollTop - view.clientHeight < 48)
  }

  return {
    selected, select, lines, clear,
    tailLines, setTailLines,
    streamState, retry,
    filter, setFilter,
    follow, setFollow, handleScroll, viewRef,
  }
}

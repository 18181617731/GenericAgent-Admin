import { apiStream } from '../lib/api'
import { decodeTerminalBase64 } from './localCmdTerminal'

const streamUrl = id => `/api/local-cmd/sessions/${encodeURIComponent(id)}/stream`

export const readLocalCmdStream = async (id, from, onEvent, signal, instanceID) => {
  const response = await apiStream(`${streamUrl(id)}?from=${from}`, { signal, instanceID })
  if (!response.body?.getReader) throw new Error('The browser does not support terminal streaming.')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let pending = ''
  while (true) {
    const result = await reader.read()
    if (result.done) break
    pending += decoder.decode(result.value, { stream: true })
    pending = consumeLocalCmdLines(pending, onEvent)
  }
  pending += decoder.decode()
  consumeLocalCmdLines(pending, onEvent)
}

const consumeLocalCmdLines = (pending, onEvent) => {
  const lines = pending.split('\n')
  const tail = lines.pop() || ''
  lines.forEach(line => parseLocalCmdEvent(line, onEvent))
  return tail
}

const parseLocalCmdEvent = (line, onEvent) => {
  const value = line.trim()
  if (!value) return
  let event
  try { event = JSON.parse(value) } catch { return }
  if (event.type === 'data' && event.data) event.bytes = decodeTerminalBase64(event.data)
  onEvent(event)
}

export const waitForLocalCmdReconnect = signal => new Promise((resolve, reject) => {
  const timer = window.setTimeout(resolve, 800)
  signal?.addEventListener('abort', () => {
    window.clearTimeout(timer)
    reject(new DOMException('Aborted', 'AbortError'))
  }, { once: true })
})

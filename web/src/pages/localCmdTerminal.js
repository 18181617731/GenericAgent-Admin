export const LOCAL_CMD_CLIENT_BUFFER_LIMIT = 2 * 1024 * 1024

const asBytes = value => {
  if (ArrayBuffer.isView(value)) return new Uint8Array(value.buffer, value.byteOffset, value.byteLength)
  if (value instanceof Uint8Array) return value
  if (value instanceof ArrayBuffer) return new Uint8Array(value)
  return new TextEncoder().encode(String(value || ''))
}

export const createTerminalBuffer = () => ({ chunks: [], totalBytes: 0 })

export const appendTerminalChunk = (buffer, value, limit = LOCAL_CMD_CLIENT_BUFFER_LIMIT) => {
  const chunk = asBytes(value)
  if (!chunk.length) return buffer
  buffer.chunks.push(chunk.slice())
  buffer.totalBytes += chunk.length
  while (buffer.totalBytes > limit && buffer.chunks.length) {
    const overflow = buffer.totalBytes - limit
    const first = buffer.chunks[0]
    if (first.length <= overflow) {
      buffer.chunks.shift()
      buffer.totalBytes -= first.length
      continue
    }
    buffer.chunks[0] = first.slice(overflow)
    buffer.totalBytes -= overflow
  }
  return buffer
}

export const clearTerminalBuffer = buffer => {
  buffer.chunks.length = 0
  buffer.totalBytes = 0
  return buffer
}

export const encodeTerminalInput = value => new TextEncoder().encode(String(value ?? ''))

export const decodeTerminalBase64 = encoded => {
  const binary = globalThis.atob(encoded || '')
  return Uint8Array.from(binary, char => char.charCodeAt(0))
}

export const encodeTerminalBase64 = data => {
  const bytes = asBytes(data)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return globalThis.btoa(binary)
}

const TERMINAL_SHORTCUTS = {
  Tab: '\t', Escape: '\x1b', 'Ctrl+C': '\x03', 'Ctrl+L': '\x0c',
  ArrowUp: '\x1b[A', ArrowDown: '\x1b[B', ArrowLeft: '\x1b[D', ArrowRight: '\x1b[C',
}

export const terminalShortcutBytes = key => encodeTerminalInput(TERMINAL_SHORTCUTS[key] || '')

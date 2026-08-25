export const LOCAL_CMD_CLIENT_BUFFER_LIMIT = 2 * 1024 * 1024

const ANSI_SEQUENCE = /\x1b\][^\x07]*(?:\x07|\x1b\\)|\x1b\[[0-?]*[ -/]*[@-~]|\x1b[@-_]/g

export const stripTerminalAnsi = value => String(value || '').replace(ANSI_SEQUENCE, sequence => {
  if (/^\x1b\[\d+;1H$/.test(sequence)) return '\n'
  return /\x1b\[(?:2|3)J|\x1b\[H/.test(sequence) ? '\f' : ''
})

export const appendTerminalText = (previous, incoming, terminalState = { pendingCR: false }) => {
  let output = previous || ''
  let pendingCR = terminalState.pendingCR
  terminalState.pendingCR = false
  for (const char of stripTerminalAnsi(incoming)) {
    if (pendingCR) {
      pendingCR = false
      if (char === '\n') {
        output += '\n'
        continue
      }
      output = output.slice(0, output.lastIndexOf('\n') + 1)
    }
    if (char === '\f') {
      output = ''
      continue
    }
    if (char === '\r') {
      pendingCR = true
      continue
    }
    if (char === '\b') {
      output = output.slice(0, -1)
      continue
    }
    if (char === '\u0000') continue
    output += char
  }
  terminalState.pendingCR = pendingCR
  return output.length > LOCAL_CMD_CLIENT_BUFFER_LIMIT
    ? output.slice(-LOCAL_CMD_CLIENT_BUFFER_LIMIT)
    : output
}

export const decodeTerminalBase64 = encoded => {
  const binary = globalThis.atob(encoded || '')
  return Uint8Array.from(binary, char => char.charCodeAt(0))
}

export const encodeTerminalBase64 = data => {
  const bytes = data instanceof Uint8Array
    ? data
    : ArrayBuffer.isView(data)
      ? new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
      : new TextEncoder().encode(String(data || ''))
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return globalThis.btoa(binary)
}

export const terminalShortcutBytes = key => ({
  Tab: '\t', Escape: '\x1b', 'Ctrl+C': '\x03', 'Ctrl+L': '\x0c', ArrowUp: '\x1b[A', ArrowDown: '\x1b[B',
  ArrowLeft: '\x1b[D', ArrowRight: '\x1b[C',
})[key] || ''

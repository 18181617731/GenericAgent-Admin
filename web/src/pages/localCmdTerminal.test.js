import { describe, expect, test } from 'vitest'
import {
  appendTerminalChunk,
  clearTerminalBuffer,
  createTerminalBuffer,
  encodeTerminalInput,
  terminalShortcutBytes,
} from './localCmdTerminal.js'

const bytes = value => Array.from(value)

describe('remote CMD terminal byte buffer', () => {
  test('preserves ANSI cursor and clear sequences as raw bytes', () => {
    const buffer = createTerminalBuffer()
    const raw = new TextEncoder().encode('\x1b[2J\x1b[Hcodex\x1b[10;1H')

    appendTerminalChunk(buffer, raw)

    expect(bytes(buffer.chunks[0])).toEqual(bytes(raw))
    expect(buffer.totalBytes).toBe(raw.length)
  })

  test('caps replay bytes without decoding or stripping VT data', () => {
    const buffer = createTerminalBuffer()
    const raw = Uint8Array.from([0x1b, 0x5b, 0x32, 0x4a, 0xff, 0x00, 0x41])

    appendTerminalChunk(buffer, raw, 5)

    expect(bytes(buffer.chunks.flatMap(chunk => Array.from(chunk)))).toEqual([0x32, 0x4a, 0xff, 0x00, 0x41])
  })

  test('clears the local replay buffer without ending the session', () => {
    const buffer = createTerminalBuffer()
    appendTerminalChunk(buffer, new Uint8Array([65, 66]))

    clearTerminalBuffer(buffer)

    expect(buffer.chunks).toEqual([])
    expect(buffer.totalBytes).toBe(0)
  })

  test('encodes every terminal data event as UTF-8 bytes', () => {
    expect(bytes(encodeTerminalInput('中文\r'))).toEqual([0xe4, 0xb8, 0xad, 0xe6, 0x96, 0x87, 0x0d])
  })

  test('keeps mobile shortcut bytes compatible with a Windows console', () => {
    expect(bytes(terminalShortcutBytes('Tab'))).toEqual([0x09])
    expect(bytes(terminalShortcutBytes('Ctrl+C'))).toEqual([0x03])
    expect(bytes(terminalShortcutBytes('Escape'))).toEqual([0x1b])
    expect(bytes(terminalShortcutBytes('ArrowUp'))).toEqual([0x1b, 0x5b, 0x41])
  })
})

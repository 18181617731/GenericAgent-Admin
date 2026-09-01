import { describe, expect, test, vi } from 'vitest'
import { handleLocalCmdEvent, localCmdSessionStorageKey, LOCAL_CMD_SESSION_STORAGE_KEY } from './localCmdController.js'
import { createTerminalBuffer } from './localCmdTerminal.js'

const eventState = () => ({
  streamSeq: { current: 3 },
  terminalBuffer: { current: createTerminalBuffer() },
  setTerminalRevision: vi.fn(),
  sessionStatus: { current: 'running' },
  setSession: vi.fn(),
  setConnection: vi.fn(),
  setNotice: vi.fn(),
})

describe('remote CMD stream cursor', () => {
  test('does not treat sync sequence as consumed replay data', () => {
    const state = eventState()

    handleLocalCmdEvent({ type: 'sync', seq: 8, status: 'running' }, state)
    expect(state.streamSeq.current).toBe(3)

    handleLocalCmdEvent({ type: 'data', seq: 4, bytes: new Uint8Array([65]) }, state)
    expect(state.streamSeq.current).toBe(4)
  })

  test('forwards raw data bytes to the terminal buffer without text normalization', () => {
    const state = eventState()
    const raw = new Uint8Array([0x1b, 0x5b, 0x32, 0x4a, 0xff, 0x00])

    handleLocalCmdEvent({ type: 'data', seq: 4, bytes: raw }, state)

    expect(Array.from(state.terminalBuffer.current.chunks[0])).toEqual(Array.from(raw))
    expect(state.setTerminalRevision).toHaveBeenCalledTimes(1)
    expect(state.setSession).not.toHaveBeenCalled()
  })
})

describe('remote CMD instance storage', () => {
  test('keeps the default key backwards-compatible and scopes other instances', () => {
    expect(localCmdSessionStorageKey('')).toBe(LOCAL_CMD_SESSION_STORAGE_KEY)
    expect(localCmdSessionStorageKey('default')).toBe(LOCAL_CMD_SESSION_STORAGE_KEY)
    expect(localCmdSessionStorageKey('beta')).toBe(`${LOCAL_CMD_SESSION_STORAGE_KEY}:beta`)
  })
})

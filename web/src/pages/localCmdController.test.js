import { describe, expect, test, vi } from 'vitest'
import { handleLocalCmdEvent } from './localCmdController.js'

const eventState = () => ({
  streamSeq: { current: 3 },
  decoder: { current: new TextDecoder() },
  terminalState: { current: { pendingCR: false } },
  sessionStatus: { current: 'running' },
  setSession: vi.fn(),
  setOutput: vi.fn(),
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
})

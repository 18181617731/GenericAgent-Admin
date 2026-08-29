import React from 'react'
import { act, cleanup, render, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, test, vi } from 'vitest'

const mocks = vi.hoisted(() => ({ terminals: [], observers: [] }))

vi.mock('@xterm/xterm', () => ({
  Terminal: class {
    constructor() {
      this.cols = 100
      this.rows = 30
      this.writes = []
      this.clearCount = 0
      this.disposed = false
      mocks.terminals.push(this)
    }

    loadAddon(addon) { this.addon = addon }
    open(host) { this.host = host }
    onData(callback) { this.dataCallback = callback; return { dispose: () => { this.dataDisposed = true } } }
    write(data) { this.writes.push(data) }
    clear() { this.clearCount += 1 }
    focus() { this.focused = true }
    dispose() { this.disposed = true }
  },
}))

vi.mock('@xterm/addon-fit', () => ({
  FitAddon: class {
    fit() {}
  },
}))

vi.stubGlobal('ResizeObserver', class {
  constructor(callback) { this.callback = callback; mocks.observers.push(this) }
  observe(target) { this.target = target }
  disconnect() { this.disconnected = true }
})

import { RemoteCmdTerminal } from './RemoteCmdTerminal.jsx'

afterEach(() => {
  cleanup()
  mocks.terminals.length = 0
  mocks.observers.length = 0
  vi.useRealTimers()
})

describe('RemoteCmdTerminal', () => {
  test('writes raw VT bytes once and forwards each onData event', async () => {
    const raw = Uint8Array.from([0x1b, 0x5b, 0x32, 0x4a, 0xff])
    const onData = vi.fn()
    render(<RemoteCmdTerminal chunks={[raw]} revision={1} onData={onData} />)

    await waitFor(() => expect(mocks.terminals[0].writes).toHaveLength(1))
    expect(Array.from(mocks.terminals[0].writes[0])).toEqual(Array.from(raw))
    mocks.terminals[0].dataCallback('c')
    mocks.terminals[0].dataCallback('\x1b[A')
    expect(onData.mock.calls).toEqual([['c'], ['\x1b[A']])
  })

  test('debounces ResizeObserver changes and disposes terminal resources', async () => {
    vi.useFakeTimers()
    const onResize = vi.fn()
    const view = render(<RemoteCmdTerminal onResize={onResize} />)

    await act(async () => { vi.runOnlyPendingTimers() })
    mocks.observers[0].callback()
    mocks.observers[0].callback()
    await act(async () => { vi.runOnlyPendingTimers() })
    expect(onResize).toHaveBeenCalledTimes(1)

    view.rerender(<RemoteCmdTerminal clearToken={1} onResize={onResize} />)
    expect(mocks.terminals[0].clearCount).toBe(1)
    view.unmount()
    expect(mocks.terminals[0].dataDisposed).toBe(true)
    expect(mocks.terminals[0].disposed).toBe(true)
    expect(mocks.observers[0].disconnected).toBe(true)
  })
})

import React from 'react'
import { afterEach, describe, expect, it } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { AppDialogHost } from './AppDialogHost'
import { confirmDanger, showAppAlert } from '../lib/danger'

afterEach(() => {
  cleanup()
  document.documentElement.lang = ''
})

const requestInsideAct = async factory => {
  let pending
  await act(async () => {
    pending = factory()
    await Promise.resolve()
  })
  return { pending }
}

describe('AppDialogHost', () => {
  it('renders confirmations in-app and preserves cancellation', async () => {
    document.documentElement.lang = 'en'
    const user = userEvent.setup()
    render(<AppDialogHost />)

    const { pending } = await requestInsideAct(() => confirmDanger('files-delete', 'Delete this file?'))

    expect(await screen.findByRole('dialog')).not.toBeNull()
    expect(screen.getByText('files-delete')).not.toBeNull()
    expect(screen.getByText('Delete this file?')).not.toBeNull()
    await user.click(screen.getByRole('button', { name: 'Cancel' }))

    await expect(pending).resolves.toBe(false)
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('queues alerts behind confirmations and uses one close action', async () => {
    document.documentElement.lang = 'en'
    const user = userEvent.setup()
    render(<AppDialogHost />)

    const { pending: first } = await requestInsideAct(() => confirmDanger('first-op', 'Continue?'))
    const second = showAppAlert('Open failed', { operation: 'chat-file-open' })

    expect(await screen.findByText('Continue?')).not.toBeNull()
    await user.click(screen.getByRole('button', { name: 'Confirm' }))
    await expect(first).resolves.toBe(true)

    expect(await screen.findByText('Open failed')).not.toBeNull()
    expect(screen.getByText('chat-file-open')).not.toBeNull()
    const cancelButton = screen.getByRole('button', { name: 'Cancel' })
    expect(cancelButton.className).toContain('app-dialog-cancel-hidden')
    await user.click(screen.getByRole('button', { name: 'Got it' }))
    await expect(second).resolves.toBe(true)
  })

  it('cannot drop the next queued dialog when the first action settles twice', async () => {
    document.documentElement.lang = 'en'
    render(<AppDialogHost />)

    const { pending: first } = await requestInsideAct(() => confirmDanger('first-op', 'First?'))
    const second = confirmDanger('second-op', 'Second?')
    const confirmButton = await screen.findByRole('button', { name: 'Confirm' })

    act(() => {
      fireEvent.click(confirmButton)
      fireEvent.click(confirmButton)
    })

    await expect(first).resolves.toBe(true)
    expect(await screen.findByText('Second?')).not.toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    await expect(second).resolves.toBe(false)
  })
})

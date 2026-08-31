import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'

vi.mock('./pages/KeychainPage', () => ({
  KeychainPage: ({ text }) => <div data-testid="keychain-page">{text.keychain.title}</div>,
}))

import { ChatKeychainDialog, ComposerActions } from './ChatApp.jsx'

afterEach(() => cleanup())

describe('chat keychain access', () => {
  test('exposes keychain from the composer action menu', () => {
    const onKeychain = vi.fn()
    render(<ComposerActions
      onAttach={vi.fn()}
      onCommands={vi.fn()}
      onSystemPrompt={vi.fn()}
      onKeychain={onKeychain}
      commandsOpen={false}
      keychainOpen={false}
      systemPromptActive={false}
      systemPromptLabel=""
    />)

    fireEvent.click(screen.getByRole('button', { name: /密钥管理|keychain/i }))
    fireEvent.click(screen.getByRole('menuitem', { name: /密钥管理|keychain/i }))

    expect(onKeychain).toHaveBeenCalledOnce()
    expect(screen.queryByRole('menu')).toBeNull()
  })

  test('renders an accessible dialog and closes it with Escape', () => {
    const onClose = vi.fn()
    render(<ChatKeychainDialog open onClose={onClose}/>)

    expect(screen.getByRole('dialog', { name: /密钥管理|keychain/i })).toBeTruthy()
    expect(screen.getByTestId('keychain-page')).toBeTruthy()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })

  test('returns focus to the persistent composer trigger after closing', async () => {
    function FocusHarness() {
      const [open, setOpen] = React.useState(true)
      const triggerRef = React.useRef(null)
      return <>
        <button ref={triggerRef}>Composer actions</button>
        <ChatKeychainDialog
          open={open}
          onClose={() => setOpen(false)}
          returnFocusRef={triggerRef}
        />
      </>
    }

    render(<FocusHarness/>)
    fireEvent.keyDown(document, { key: 'Escape' })

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).toBeNull()
      expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Composer actions' }))
    })
  })

  test('only closes on the backdrop itself', () => {
    const onClose = vi.fn()
    render(<ChatKeychainDialog open onClose={onClose}/>)

    const dialog = screen.getByRole('dialog', { name: /密钥管理|keychain/i })
    fireEvent.mouseDown(dialog)
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.mouseDown(dialog.parentElement)
    expect(onClose).toHaveBeenCalledOnce()
  })
})

// @vitest-environment jsdom

import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { isWorldlinePickerResult, WorldlineRestoreDialog, worldlineRestoreCommand } from './ChatApp.jsx'

afterEach(() => cleanup())

describe('worldline restore-point chooser', () => {
  test('builds the complete official restore command from all selections', () => {
    expect(worldlineRestoreCommand(' node-42 ', 'conversation', 'before')).toBe('/worldline restore node-42 conversation before')
    expect(worldlineRestoreCommand('node-42', 'both', 'at')).toBe('/worldline restore node-42 both at')
    expect(worldlineRestoreCommand('')).toBe('')
  })

  test('selects node, scope, and target before confirming without submitting itself', () => {
    const onSelect = vi.fn()
    render(<WorldlineRestoreDialog nodes={[
      { id:'node-1', ordinal:3, title:'Before refactor' },
      { id:'node-2', ordinal:4, title:'After refactor' },
    ]} onClose={vi.fn()} onSelect={onSelect}/>)

    expect(screen.getByRole('dialog', { name:'选择回退点' })).toBeTruthy()
    expect(screen.getByText('node-2')).toBeTruthy()
    expect(screen.getByRole('button', { name:'确认并填入命令' }).disabled).toBe(true)

    fireEvent.click(screen.getByRole('button', { name:/After refactor/ }))
    fireEvent.click(screen.getByRole('button', { name:'仅对话' }))
    fireEvent.click(screen.getByRole('button', { name:'节点之前' }))
    expect(onSelect).not.toHaveBeenCalled()

    const confirm = screen.getByRole('button', { name:'确认并填入命令' })
    expect(confirm.disabled).toBe(false)
    fireEvent.click(confirm)
    expect(onSelect).toHaveBeenCalledTimes(1)
    expect(onSelect).toHaveBeenCalledWith('node-2', 'conversation', 'before')
  })

  test('keeps zero-node feedback in chat but presents populated lists only in the picker', () => {
    expect(isWorldlinePickerResult({ command:'worldline', action:'list', tree:{ nodes:[{ id:'node-1' }] } })).toBe(true)
    expect(isWorldlinePickerResult({ command:'worldline', action:'list', tree:{ nodes:[] } })).toBe(false)
    expect(isWorldlinePickerResult({ command:'worldline', action:'restore', tree:{ nodes:[{ id:'node-1' }] } })).toBe(false)
  })

  test('closes with Escape', () => {
    const onClose = vi.fn()
    render(<WorldlineRestoreDialog nodes={[]} onClose={onClose} onSelect={vi.fn()}/>)
    fireEvent.keyDown(window, { key:'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

// @vitest-environment jsdom

import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { isWorldlinePickerResult, WorldlinePanel, WorldlineRestoreDialog, worldlineRestoreCommand } from './ChatApp.jsx'

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

  test('marks nodes with untracked external changes in the branch panel', () => {
    const state = { available:true, nodes:[
      { id:'node-1', parent_id:null, ordinal:1, title:'干净节点', mapping_status:'mapped' },
      { id:'node-2', parent_id:'node-1', ordinal:2, title:'带外部改动', mapping_status:'mapped',
        untracked_changes:true, untracked_files:['notes.txt'] },
    ], current_path:['node-1','node-2'] }
    render(<WorldlinePanel state={state} loading={false} switchingId={null} disabled={false}
      onClose={vi.fn()} onRefresh={vi.fn()} onSwitch={vi.fn()}/>)
    const badges = screen.getAllByLabelText('外部改动')
    expect(badges.length).toBe(1)
    expect(badges[0].title).toContain('notes.txt')
  })

  test('explains missing worldline records without exposing a technical status code', () => {
    render(<WorldlinePanel state={{ available:false, degraded_reason:'missing' }} loading={false} switchingId="" disabled={false}
      onClose={vi.fn()} onRefresh={vi.fn()} onSwitch={vi.fn()}/>)
    expect(screen.getByText('当前会话还没有可用的世界线记录。完成一轮成功对话后，系统会自动创建节点。')).toBeTruthy()
    expect(screen.queryByText('missing')).toBeNull()
  })

  test('closes with Escape', () => {
    const onClose = vi.fn()
    render(<WorldlineRestoreDialog nodes={[]} onClose={onClose} onSelect={vi.fn()}/>)
    fireEvent.keyDown(window, { key:'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})

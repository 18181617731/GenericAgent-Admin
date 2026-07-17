import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { ChatMessage, WorldlineNavigator, worldlineForSession, worldlineLoaded, worldlineLoadStarted, worldlineOwnsMappedNode } from './ChatApp.jsx'

globalThis.React = React

afterEach(() => cleanup())

const tree = {
  head: 'child',
  current_path: ['root', 'child'],
  nodes: [
    { id:'root', parent_id:null, depth:0, ordinal:0, title:'Root', mapping_status:'mapped' },
    { id:'child', parent_id:'root', depth:1, ordinal:0, title:'Current child', mapping_status:'mapped' },
    { id:'sibling', parent_id:'root', depth:1, ordinal:1, title:'Sibling branch', mapping_status:'mapped' },
    { id:'legacy', parent_id:'root', depth:1, ordinal:2, title:'Legacy branch', mapping_status:'legacy' },
  ],
}

const renderNav = (state, props = {}) => render(<WorldlineNavigator state={state} onRefresh={props.onRefresh || vi.fn()} onSwitch={props.onSwitch || vi.fn()} disabled={props.disabled || false} />)

describe('worldline session state', () => {
  test('arms a new session before accepting its response and still rejects stale responses', () => {
    const previous = { sid:'old', status:'empty', data:null, error:null, switchingNodeId:'' }
    const loading = worldlineLoadStarted(previous, 'new')
    expect(loading).toEqual({ sid:'new', status:'loading', data:null, error:null, switchingNodeId:'' })

    const ready = worldlineLoaded(loading, tree, 'new')
    expect(ready.status).toBe('ready')
    expect(ready.data.head).toBe('child')
    expect(worldlineLoaded(ready, { ...tree, head:'root' }, 'old')).toBe(ready)
  })

  test('never exposes another session topology during a sid transition', () => {
    const oldReady = { sid:'old', status:'ready', data:tree, error:null, switchingNodeId:'' }
    expect(worldlineForSession(oldReady, 'new')).toEqual({ sid:'new', status:'loading', data:null, error:null, switchingNodeId:'' })
    expect(worldlineForSession(oldReady, '')).toEqual({ sid:'', status:'idle', data:null, error:null, switchingNodeId:'' })
    expect(worldlineForSession(oldReady, 'old')).toBe(oldReady)
  })

  test('only accepts mapped nodes owned by the active session topology', () => {
    const ready = { sid:'old', status:'ready', data:tree, error:null, switchingNodeId:'' }
    expect(worldlineOwnsMappedNode(ready, 'old', 'sibling')).toBe(true)
    expect(worldlineOwnsMappedNode(ready, 'new', 'sibling')).toBe(false)
    expect(worldlineOwnsMappedNode(ready, 'old', 'legacy')).toBe(false)
    expect(worldlineOwnsMappedNode(ready, 'old', 'missing')).toBe(false)
  })
})

describe('persistent worldline navigation', () => {
  test('renders nested hydrated path and switches to a mapped sibling', () => {
    const onSwitch = vi.fn()
    renderNav({ status:'ready', data:tree, error:'', switchingNodeId:'' }, { onSwitch })
    expect(screen.getByText('4 个节点 · 当前路径 2 层')).toBeTruthy()
    expect(screen.getByText('Current child').closest('button').getAttribute('aria-current')).toBe('step')
    expect(screen.getByText('Legacy branch').closest('button').disabled).toBe(true)
    fireEvent.click(screen.getByText('Sibling branch').closest('button'))
    expect(onSwitch).toHaveBeenCalledWith('sibling')
  })

  test.each([
    ['loading', '正在读取分支拓扑'],
    ['unavailable', '当前运行环境未提供分支导航'],
    ['degraded', '分支服务暂不可用：legacy session'],
    ['empty', '发送消息后，这里会显示可切换的对话路径'],
  ])('renders explicit %s state', (status, text) => {
    renderNav({ status, data:status === 'degraded' ? { nodes:[], current_path:[], degraded_reason:'legacy session' } : null, error:'', switchingNodeId:'' })
    expect(screen.getByText(text, { exact:false })).toBeTruthy()
  })

  test('preserves stale topology, reports refresh error, and disables controls while switching', () => {
    renderNav({ status:'stale-error', data:tree, error:'offline', switchingNodeId:'sibling' })
    expect(screen.getByRole('alert').textContent).toContain('继续显示上次路径')
    expect(screen.getByText('Root')).toBeTruthy()
    expect(screen.getByLabelText('刷新对话分支').disabled).toBe(true)
    expect(screen.getByText('Sibling branch').closest('button').disabled).toBe(true)
  })

  test('disables mapped navigation while the conversation is running', () => {
    renderNav({ status:'ready', data:tree, error:'', switchingNodeId:'' }, { disabled:true })
    expect(screen.getByText('Sibling branch').closest('button').disabled).toBe(true)
  })
})

describe('worldline action separation', () => {
  test('message sibling controls switch conversation versions accessibly', () => {
    const onSwitch = vi.fn()
    render(<ChatMessage message={{ id:'m1', role:'assistant', content:'answer' }} version={{ index:2, total:3, previous_node_id:'n1', next_node_id:'n3' }} onSwitchVersion={onSwitch} />)
    fireEvent.click(screen.getByLabelText('上一个消息版本'))
    fireEvent.click(screen.getByLabelText('下一个消息版本'))
    expect(onSwitch.mock.calls).toEqual([['n1'], ['n3']])
  })
})

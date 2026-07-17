import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { ChatMessage, WorldlineNavigator, projectWorldline, worldlineForSession, worldlineLoaded, worldlineLoadStarted, worldlineOwnsMappedNode } from './ChatApp.jsx'

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

describe('worldline branch-depth projection', () => {
  test('keeps a continuous path on one track and only indents at branch points', () => {
    const rows = projectWorldline([
      { id:'root', parent_id:null, ordinal:0 },
      { id:'turn-2', parent_id:'root', ordinal:0 },
      { id:'choice-a', parent_id:'turn-2', ordinal:0 },
      { id:'choice-b', parent_id:'turn-2', ordinal:1 },
      { id:'after-a', parent_id:'choice-a', ordinal:0 },
      { id:'deep-a', parent_id:'after-a', ordinal:0 },
      { id:'deep-b', parent_id:'after-a', ordinal:1 },
    ])
    const depth = Object.fromEntries(rows.map(row => [row.id, row.branchDepth]))
    expect(depth).toEqual({ root:0, 'turn-2':0, 'choice-a':1, 'after-a':1, 'deep-a':2, 'deep-b':2, 'choice-b':1 })
    expect(rows.filter(row => ['choice-a', 'choice-b'].includes(row.id)).map(row => row.siblingCount)).toEqual([2, 2])
  })
})

describe('persistent worldline navigation', () => {
  test('renders branch-level tracks and switches to a mapped sibling', () => {
    const onSwitch = vi.fn()
    renderNav({ status:'ready', data:tree, error:'', switchingNodeId:'' }, { onSwitch })
    expect(screen.getByText('4 条记录')).toBeTruthy()
    expect(screen.getByText('1 处分叉')).toBeTruthy()
    expect(screen.getByText('Root').closest('button').getAttribute('data-branch-depth')).toBe('0')
    expect(screen.getByText('Current child').closest('button').getAttribute('data-branch-depth')).toBe('1')
    expect(screen.getByText('Sibling branch').closest('button').getAttribute('data-branch-depth')).toBe('1')
    expect(screen.getByText('Current child').closest('button').getAttribute('aria-current')).toBe('step')
    expect(screen.getByText('Legacy branch').closest('button').disabled).toBe(true)
    fireEvent.click(screen.getByText('Sibling branch').closest('button'))
    expect(onSwitch).toHaveBeenCalledWith('sibling')
  })

  test('filters long histories by content and clears the query', () => {
    renderNav({ status:'ready', data:tree, error:'', switchingNodeId:'' })
    fireEvent.change(screen.getByLabelText('搜索对话分支'), { target:{ value:'Sibling' } })
    expect(screen.getByText('Sibling branch')).toBeTruthy()
    expect(screen.queryByText('Current child')).toBeNull()
    fireEvent.click(screen.getByLabelText('清空分支搜索'))
    expect(screen.getByText('Current child')).toBeTruthy()
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

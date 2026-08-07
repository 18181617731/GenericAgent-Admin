import test from 'node:test'
import assert from 'node:assert/strict'
import { filterTodoItems, normalizeTodoOverview, todoItemsForModule, todoModuleLabel, todoStatusLabel } from './todos.js'

const payload = {
  source_exists: true,
  total: 3,
  open: 2,
  completed: 1,
  items: [
    { id: 'a', title: '调度修复', module: 'tasks', status: 'queued', round: 'R1', priority: 'P0' },
    { id: 'b', title: '模型配置', module: 'models', status: 'needs_sync', summary: '检查模型状态' },
    { id: 'c', title: '已归档', module: 'files', status: 'completed' },
  ],
  modules: [{ module: 'tasks', total: 1, open: 1, completed: 0 }],
}

test('normalizes TODO payload and preserves actionable status fields', () => {
  const overview = normalizeTodoOverview(payload)
  assert.equal(overview.items.length, 3)
  assert.equal(overview.items[0].sourcePath, 'temp/TODO.txt')
  assert.equal(overview.items[1].status, 'needs_sync')
  assert.equal(overview.items[2].module, 'files')
  assert.equal(todoItemsForModule(overview, 'tasks').length, 1)
  assert.equal(overview.modules[0].needsSync, 0)
})

test('filters open and completed TODOs independently and searches context', () => {
  const overview = normalizeTodoOverview(payload)
  assert.deepEqual(filterTodoItems(overview.items).map(item => item.id), ['a', 'b'])
  assert.deepEqual(filterTodoItems(overview.items, { showCompleted: true }).map(item => item.id), ['c'])
  assert.deepEqual(filterTodoItems(overview.items, { query: '模型' }).map(item => item.id), ['b'])
})

test('uses readable module and status labels with safe fallbacks', () => {
  assert.equal(todoModuleLabel('tasks'), '定时任务')
  assert.equal(todoModuleLabel('tasks', 'en'), 'Scheduled tasks')
  assert.equal(todoStatusLabel('queued'), '已批准，等待执行')
  assert.equal(todoStatusLabel('unknown'), '待确认')
})

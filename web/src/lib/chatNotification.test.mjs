import test from 'node:test'
import assert from 'node:assert/strict'
import { buildChatNotification, latestUserPrompt } from './chatNotification.js'

test('finds the latest readable user prompt for resumed conversations', () => {
  assert.equal(latestUserPrompt([{ role: 'user', content: '第一轮' }, { role: 'assistant', content: '回答' }, { role: 'user', content: '第二轮' }]), '第二轮')
  assert.equal(latestUserPrompt([{ role: 'assistant', content: '回答' }]), '')
})

test('uses a readable chat title and a question plus short code to distinguish duplicate titles', () => {
  const result = buildChatNotification({
    session: { id: 'session-abc123', title: '北京时间查询', project_mode: '日常问答' },
    prompt: '请告诉我北京时间现在几点了',
  })
  assert.equal(result.title, '对话已完成：北京时间查询')
  assert.match(result.message, /“北京时间查询”已完成回复。/)
  assert.match(result.message, /刚才的问题：请告诉我北京时间现在几点了/)
  assert.match(result.message, /项目：日常问答/)
  assert.match(result.message, /对话尾号：abc123/)
  assert.doesNotMatch(result.message, /session-abc123/)
})

test('falls back safely for a new session and formats failures', () => {
  const result = buildChatNotification({ session: { id: 'new-987654', title: '新会话' }, prompt: '检查服务状态', status: 'failed', error: '网络超时' })
  assert.equal(result.title, '对话执行失败：检查服务状态')
  assert.match(result.message, /“检查服务状态”未能完成：网络超时。/)
  assert.match(result.message, /刚才的问题：检查服务状态/)
  assert.match(result.message, /对话尾号：987654/)
  const fallback = buildChatNotification({ sessionId: 'new-123456', status: 'completed' })
  assert.equal(fallback.title, '对话已完成：新会话')
  assert.match(fallback.message, /范围：最近一轮对话/)
})

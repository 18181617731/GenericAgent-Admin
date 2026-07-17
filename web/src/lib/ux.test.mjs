import test from 'node:test'
import assert from 'node:assert/strict'
import {
  feedbackTone,
  filterModelProviderGroups,
  modelGroupStats,
  shouldConfirmFileReplacement,
  updateStatusPresentation,
} from './ux.js'

test('feedback tone uses explicit semantics and sensible message inference', () => {
  assert.equal(feedbackTone('模型检测失败'), 'error')
  assert.equal(feedbackTone('配置已保存'), 'success')
  assert.equal(feedbackTone('正在加载'), 'info')
  assert.equal(feedbackTone('anything', 'warning'), 'warning')
})

test('model group filtering matches provider or individual model names', () => {
  const groups = [
    { value: 'a', label: 'OpenAI', models: [{ value: 1, label: 'gpt-5' }, { value: 2, label: 'o3' }] },
    { value: 'b', label: 'Anthropic', models: [{ value: 3, label: 'claude-sonnet' }] },
  ]
  assert.deepEqual(filterModelProviderGroups(groups, 'open'), [groups[0]])
  assert.deepEqual(filterModelProviderGroups(groups, 'sonnet'), [{ ...groups[1], models: [groups[1].models[0]] }])
  assert.deepEqual(modelGroupStats(groups), { providers: 2, models: 3 })
})

test('update presentation distinguishes interrupted and ordinary failed upgrades', () => {
  const interrupted = updateStatusPresentation({ stage: 'error', message: '升级在 downloading 阶段中断', error: '进程重启前未完成', id: 'u1' })
  assert.equal(interrupted.interrupted, true)
  assert.equal(interrupted.actionLabel, '重新开始升级')
  assert.match(interrupted.detail, /任务：u1/)

  const failed = updateStatusPresentation({ stage: 'error', error: 'checksum mismatch' })
  assert.equal(failed.failed, true)
  assert.equal(failed.actionLabel, '重试升级')
})

test('file replacement confirmation is only required for dirty content moving to another file', () => {
  assert.equal(shouldConfirmFileReplacement({ dirty: false, loadedPath: 'a', nextPath: 'b' }), false)
  assert.equal(shouldConfirmFileReplacement({ dirty: true, loadedPath: 'a', nextPath: 'a' }), false)
  assert.equal(shouldConfirmFileReplacement({ dirty: true, loadedPath: 'a', nextPath: 'b' }), true)
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { localizeAutonomousApprovalValue } from './autonomousCopy.js'

test('localizes autonomous approval enum values in Chinese', () => {
  assert.equal(localizeAutonomousApprovalValue('needs_approval', 'zh', 'reviewDecision'), '需要审批')
  assert.equal(localizeAutonomousApprovalValue('high', 'zh', 'reviewConfidence'), '高')
  assert.equal(localizeAutonomousApprovalValue('human review required', 'zh', 'risk'), '需要人工复核')
})

test('localizes generated review reasons while preserving separators', () => {
  const reason = 'report is blocked; the proposed source change is not confirmed as implemented; model review unavailable: model review in progress; conservative rule retained'
  assert.equal(localizeAutonomousApprovalValue(reason, 'zh', 'reviewReason'), '报告处于阻塞状态; 拟议源码变更尚未确认实施; 模型审核不可用: 模型审核进行中; 已保留保守规则')
})

test('keeps English approval values unchanged', () => {
  const value = 'report requires human approval'
  assert.equal(localizeAutonomousApprovalValue(value, 'en', 'status'), value)
})

test('localizes report titles without changing technical source paths', () => {
  assert.equal(localizeAutonomousApprovalValue('R49_complete_task approval review', 'zh', 'title'), 'R49_完成任务 审批 复核')
})

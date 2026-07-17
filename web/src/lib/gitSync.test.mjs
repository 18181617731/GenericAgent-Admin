import test from 'node:test'
import assert from 'node:assert/strict'
import { gitSyncPresentation } from './gitSync.js'

test('git sync distinguishes remote latest from fully synchronized', () => {
  assert.deepEqual(gitSyncPresentation({ remote_checked: true, synchronized: true }), {
    state: 'synced', label: '已同步', summary: '当前分支已与 origin 完全同步', canSync: false,
  })
  const pending = gitSyncPresentation({ remote_checked: true, dirty: true, changed_files: 3, ahead: 2, strategy_available: true })
  assert.equal(pending.state, 'pending')
  assert.equal(pending.canSync, true)
  assert.match(pending.summary, /3 个本地变更/)
  assert.match(pending.summary, /2 个待推送提交/)
})

test('git sync blocks conflicts and reports remote commits', () => {
  assert.equal(gitSyncPresentation({ remote_checked: true, conflicts: true }).canSync, false)
  const behind = gitSyncPresentation({ remote_checked: true, behind: 4, strategy_available: true })
  assert.equal(behind.label, '远端领先 4')
  assert.equal(behind.canSync, true)
})

test('git sync blocks a branch that does not track origin', () => {
  const result = gitSyncPresentation({
    remote_checked: true,
    tracking_matches_origin: false,
    expected_origin: 'origin/company_ga',
    strategy_available: true,
  })
  assert.equal(result.state, 'blocked')
  assert.equal(result.canSync, false)
  assert.match(result.summary, /origin\/company_ga/)
})

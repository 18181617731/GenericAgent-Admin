import test from 'node:test'
import assert from 'node:assert/strict'
import {
  AUTORUN_FIRST_WAIT_MS,
  AUTORUN_IDLE_MS,
  autorunInitialReplyAt,
  shouldTriggerAutorun,
} from './chatAutorun.js'

test('enabling autorun schedules the first run for about one minute later', () => {
  const nowMs = 2_000_000_000
  const lastReplyAtMs = autorunInitialReplyAt(nowMs)
  assert.equal(nowMs - lastReplyAtMs, AUTORUN_IDLE_MS - AUTORUN_FIRST_WAIT_MS)
  assert.equal(shouldTriggerAutorun({ enabled:true, nowMs:nowMs + AUTORUN_FIRST_WAIT_MS, lastReplyAtMs, blocked:false }), false)
  assert.equal(shouldTriggerAutorun({ enabled:true, nowMs:nowMs + AUTORUN_FIRST_WAIT_MS + 1, lastReplyAtMs, blocked:false }), true)
})

test('subsequent autoruns wait for more than thirty idle minutes', () => {
  const lastReplyAtMs = 1_000
  assert.equal(shouldTriggerAutorun({ enabled:true, nowMs:lastReplyAtMs + AUTORUN_IDLE_MS, lastReplyAtMs, blocked:false }), false)
  assert.equal(shouldTriggerAutorun({ enabled:true, nowMs:lastReplyAtMs + AUTORUN_IDLE_MS + 1, lastReplyAtMs, blocked:false }), true)
})

test('disabled or blocked autorun never triggers', () => {
  const due = { nowMs:AUTORUN_IDLE_MS + 2, lastReplyAtMs:1 }
  assert.equal(shouldTriggerAutorun({ ...due, enabled:false, blocked:false }), false)
  assert.equal(shouldTriggerAutorun({ ...due, enabled:true, blocked:true }), false)
})

test('invalid clocks fail closed', () => {
  assert.equal(shouldTriggerAutorun({ enabled:true, nowMs:NaN, lastReplyAtMs:0, blocked:false }), false)
  assert.equal(shouldTriggerAutorun({ enabled:true, nowMs:1, lastReplyAtMs:undefined, blocked:false }), false)
})

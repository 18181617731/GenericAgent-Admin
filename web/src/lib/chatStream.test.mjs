import test from 'node:test'
import assert from 'node:assert/strict'
import { isBTWCommand, shouldFinishStreamFollow } from './chatStream.js'

test('recognizes only the dedicated btw command boundary', () => {
  assert.equal(isBTWCommand('/btw question'), true)
  assert.equal(isBTWCommand('  /btw\tquestion  '), true)
  assert.equal(isBTWCommand('/btw'), true)
  assert.equal(isBTWCommand('/btwReply question'), false)
  assert.equal(isBTWCommand('question /btw later'), false)
})


test('stream follow only stops after an empty completed replay of a finished run', () => {
  assert.equal(shouldFinishStreamFollow({ running:false, replay:true, completed:true, eventCount:0 }), true)
  assert.equal(shouldFinishStreamFollow({ running:false, replay:true, completed:true, eventCount:1 }), false)
  assert.equal(shouldFinishStreamFollow({ running:false, replay:false, completed:true, eventCount:0 }), false)
  assert.equal(shouldFinishStreamFollow({ running:false, replay:true, completed:false, eventCount:0 }), false)
  assert.equal(shouldFinishStreamFollow({ running:true, replay:true, completed:true, eventCount:0 }), false)
})

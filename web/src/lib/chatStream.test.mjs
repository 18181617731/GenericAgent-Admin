import test from 'node:test'
import assert from 'node:assert/strict'
import { isBTWCommand, mergeFinalStreamMessage, shouldFinishStreamFollow } from './chatStream.js'

test('recognizes only the dedicated btw command boundary', () => {
  assert.equal(isBTWCommand('/btw question'), true)
  assert.equal(isBTWCommand('  /btw\tquestion  '), true)
  assert.equal(isBTWCommand('/btw'), true)
  assert.equal(isBTWCommand('/btwReply question'), false)
  assert.equal(isBTWCommand('question /btw later'), false)
})

test('final stream message keeps realtime usage absent from the persisted event', () => {
  const usage = { input_tokens: 4290, output_tokens: 118 }
  const usages = [usage]
  const merged = mergeFinalStreamMessage({ model_id:'live-model', usage, usages }, { id:'final', content:'done' })
  assert.equal(merged.model_id, 'live-model')
  assert.equal(merged.usage, usage)
  assert.equal(merged.usages, usages)
})

test('stream follow only stops after an empty completed replay of a finished run', () => {
  assert.equal(shouldFinishStreamFollow({ running:false, replay:true, completed:true, eventCount:0 }), true)
  assert.equal(shouldFinishStreamFollow({ running:false, replay:true, completed:true, eventCount:1 }), false)
  assert.equal(shouldFinishStreamFollow({ running:false, replay:false, completed:true, eventCount:0 }), false)
  assert.equal(shouldFinishStreamFollow({ running:false, replay:true, completed:false, eventCount:0 }), false)
  assert.equal(shouldFinishStreamFollow({ running:true, replay:true, completed:true, eventCount:0 }), false)
})

test('authoritative final usage wins when present', () => {
  const merged = mergeFinalStreamMessage(
    { usage:{ input_tokens:1 }, usages:[{ input_tokens:1 }] },
    { usage:{ input_tokens:2 }, usages:[{ input_tokens:2 }] },
  )
  assert.equal(merged.usage.input_tokens, 2)
  assert.equal(merged.usages[0].input_tokens, 2)
})

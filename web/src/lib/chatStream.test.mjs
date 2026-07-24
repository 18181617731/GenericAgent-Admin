import test from 'node:test'
import assert from 'node:assert/strict'
import { createStreamDeltaBatcher, isBTWCommand, mergeFinalStreamMessage, shouldFinishStreamFollow } from './chatStream.js'

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


test('stream delta batcher combines chunks into one scheduled render', () => {
  const callbacks = []
  const flushed = []
  const batcher = createStreamDeltaBatcher({
    onFlush: chunk => flushed.push(chunk),
    schedule: callback => { callbacks.push(callback); return callbacks.length - 1 },
    cancel: () => {},
  })
  batcher.push('hel')
  batcher.push('lo')
  assert.equal(callbacks.length, 1)
  assert.deepEqual(flushed, [])
  callbacks[0]()
  assert.deepEqual(flushed, ['hello'])
})

test('stream delta batcher flushes pending text before terminal events', () => {
  let scheduled
  const canceled = []
  const flushed = []
  const batcher = createStreamDeltaBatcher({
    onFlush: chunk => flushed.push(chunk),
    schedule: callback => { scheduled = callback; return 7 },
    cancel: handle => canceled.push(handle),
  })
  batcher.push('final')
  batcher.flushNow()
  assert.deepEqual(canceled, [7])
  assert.deepEqual(flushed, ['final'])
  scheduled()
  assert.deepEqual(flushed, ['final'])
})


test('stream delta batcher paces a network burst across animation frames', () => {
  const callbacks = []
  const flushed = []
  const batcher = createStreamDeltaBatcher({
    onFlush: chunk => flushed.push(chunk),
    schedule: callback => { callbacks.push(callback); return callbacks.length },
    cancel: () => {},
  })
  const burst = 'x'.repeat(160)
  batcher.push(burst)
  callbacks.shift()()
  assert.equal(flushed[0].length, 20)
  assert.equal(callbacks.length, 1)
  callbacks.shift()()
  assert.equal(flushed.join('').length, 38)
  assert.equal(callbacks.length, 1)
})

test('stream delta batcher resolves drain only after all paced frames render', async () => {
  const callbacks = []
  const flushed = []
  const batcher = createStreamDeltaBatcher({
    onFlush: chunk => flushed.push(chunk),
    schedule: callback => { callbacks.push(callback); return callbacks.length },
    cancel: () => {},
  })
  batcher.push('smooth '.repeat(20))
  let drained = false
  const done = batcher.drain().then(() => { drained = true })
  await Promise.resolve()
  assert.equal(drained, false)
  while (callbacks.length) {
    callbacks.shift()()
    await Promise.resolve()
  }
  await done
  assert.equal(drained, true)
  assert.equal(flushed.join(''), 'smooth '.repeat(20))
  assert.ok(flushed.length > 1)
})

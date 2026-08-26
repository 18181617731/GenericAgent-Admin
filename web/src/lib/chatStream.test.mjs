import test from 'node:test'
import assert from 'node:assert/strict'
import { createStreamDeltaBatcher, decideStreamFollow, isBTWCommand, isLoopFollowActive, mergeFinalStreamMessage, pickResumePlaceholderId, sameStreamRun, scrollFollowAction, shouldFinishStreamFollow } from './chatStream.js'

test('scroll follow preserves auto mode when fast content growth moves the bottom away', () => {
  assert.equal(scrollFollowAction({ nearBottom: false, previousScrollTop: 320, scrollTop: 320 }), 'preserve')
  assert.equal(scrollFollowAction({ nearBottom: false, previousScrollTop: 320, scrollTop: 321 }), 'preserve')
})

test('scroll follow pauses only for upward movement and resumes at the bottom', () => {
  assert.equal(scrollFollowAction({ nearBottom: false, previousScrollTop: 320, scrollTop: 300 }), 'pause')
  assert.equal(scrollFollowAction({ nearBottom: true, previousScrollTop: 300, scrollTop: 420 }), 'resume')
})

test('scroll follow ignores the offset the app moved itself', () => {
  assert.equal(scrollFollowAction({
    nearBottom: false, previousScrollTop: 320, scrollTop: 300, programmatic: true,
  }), 'preserve')
  // A jump away from the end starts at the end, and those first pixels must
  // not be mistaken for a reader settling back into following.
  assert.equal(scrollFollowAction({
    nearBottom: true, previousScrollTop: 3850, scrollTop: 3845, programmatic: true,
  }), 'preserve')
})

test('scroll follow survives content that collapses above the reader', () => {
  // A tool card folding shut pulls the offset up without anyone scrolling; the
  // shorter page is what gives it away.
  assert.equal(scrollFollowAction({
    nearBottom: false, previousScrollTop: 900, scrollTop: 600,
    previousScrollHeight: 4000, scrollHeight: 3400,
  }), 'preserve')
  // Same move while the page keeps growing is a reader walking back up.
  assert.equal(scrollFollowAction({
    nearBottom: false, previousScrollTop: 900, scrollTop: 600,
    previousScrollHeight: 4000, scrollHeight: 4200,
  }), 'pause')
})

test('resume targets the tail placeholder and never a stale empty assistant mid-history', () => {
  const stale = { id:'stale-mid', role:'assistant', content:'' }
  const tail = { id:'tail-live', role:'assistant', content:'' }
  // The regression: a leftover empty assistant sits before the real placeholder.
  assert.equal(pickResumePlaceholderId([
    { id:'u1', role:'user', content:'hi' },
    stale,
    { id:'u2', role:'user', content:'again' },
    tail,
  ]), 'tail-live')
  // No placeholder yet (backend has not appended it): caller must create its own.
  assert.equal(pickResumePlaceholderId([{ id:'u1', role:'user', content:'hi' }]), '')
  // Tail already has content, so it is a finished message, not a placeholder.
  assert.equal(pickResumePlaceholderId([{ id:'a1', role:'assistant', content:'done' }]), '')
  assert.equal(pickResumePlaceholderId([]), '')
  assert.equal(pickResumePlaceholderId(undefined), '')
  assert.equal(pickResumePlaceholderId([{ role:'assistant', content:'' }]), '')
})

test('recognizes only the dedicated btw command boundary', () => {
  assert.equal(isBTWCommand('/btw question'), true)
  assert.equal(isBTWCommand('  /btw\tquestion  '), true)
  assert.equal(isBTWCommand('/btw'), true)
  assert.equal(isBTWCommand('/btwReply question'), false)
  assert.equal(isBTWCommand('question /btw later'), false)
})

test('final stream message keeps realtime usage and context absent from the persisted event', () => {
  const usage = { input_tokens: 4290, output_tokens: 118 }
  const usages = [usage]
  const merged = mergeFinalStreamMessage(
    { model_id:'live-model', usage, usages, ctx_chars:3800, ctx_msgs:3 },
    { id:'final', content:'done' },
  )
  assert.equal(merged.model_id, 'live-model')
  assert.equal(merged.usage, usage)
  assert.equal(merged.usages, usages)
  assert.equal(merged.ctx_chars, 3800)
  assert.equal(merged.ctx_msgs, 3)
})

test('final stream message keeps authoritative timing and falls back to streamed timing', () => {
  const merged = mergeFinalStreamMessage(
    { llm_elapsed_ms: 900, tool_elapsed_ms: 1_200, tool_live_elapsed_ms: 1_500, tool_live_active_count: 1 },
    { id:'final', content:'done', llm_elapsed_ms: 2_000, tool_elapsed_ms: 2_400 },
  )
  assert.equal(merged.llm_elapsed_ms, 2_000)
  assert.equal(merged.tool_elapsed_ms, 2_400)
  assert.equal(merged.tool_live_elapsed_ms, undefined)

  const fallback = mergeFinalStreamMessage(
    { llm_elapsed_ms: 900, tool_elapsed_ms: 1_200 },
    { id:'final', content:'done' },
  )
  assert.equal(fallback.llm_elapsed_ms, 900)
  assert.equal(fallback.tool_elapsed_ms, 1_200)
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


test('replay-mode batcher holds backlog silently then flushes instantly on beginLive', () => {
  const callbacks = []
  const flushed = []
  const batcher = createStreamDeltaBatcher({
    onFlush: chunk => flushed.push(chunk),
    schedule: callback => { callbacks.push(callback); return callbacks.length },
    cancel: () => {},
    live: false,
  })
  batcher.push('replayed '.repeat(50))
  assert.equal(callbacks.length, 0)
  assert.deepEqual(flushed, [])
  batcher.beginLive()
  assert.deepEqual(flushed, ['replayed '.repeat(50)])
  batcher.push('live-part')
  assert.equal(callbacks.length, 1)
  callbacks.shift()()
  assert.equal(flushed.join(''), 'replayed '.repeat(50) + 'live-part')
})

test('replay-mode batcher drain flushes backlog even without a sync boundary', async () => {
  const flushed = []
  const batcher = createStreamDeltaBatcher({
    onFlush: chunk => flushed.push(chunk),
    schedule: () => 1,
    cancel: () => {},
    live: false,
  })
  batcher.push('tail')
  await batcher.drain()
  assert.deepEqual(flushed, ['tail'])
})

test('beginLive is idempotent and harmless when already live', () => {
  const callbacks = []
  const flushed = []
  const batcher = createStreamDeltaBatcher({
    onFlush: chunk => flushed.push(chunk),
    schedule: callback => { callbacks.push(callback); return callbacks.length },
    cancel: () => {},
  })
  batcher.beginLive()
  batcher.push('abc')
  assert.equal(callbacks.length, 1)
  callbacks.shift()()
  assert.deepEqual(flushed, ['abc'])
})

test('loop follow remains active only for backend-hosted transitional states', () => {
  for (const status of ['waiting', 'running', 'evaluating']) {
    assert.equal(isLoopFollowActive({ enabled:true, status }), true, status)
  }
  assert.equal(isLoopFollowActive({ enabled:false, status:'running' }), false)
  assert.equal(isLoopFollowActive({ enabled:true, status:'completed' }), false)
  assert.equal(isLoopFollowActive(null), false)
})

test('run identity prefers pending id and safely falls back to start time', () => {
  assert.equal(sameStreamRun(
    { pendingId:'assistant-1', startedAtMs:10 },
    { pendingId:'assistant-1', startedAtMs:99 },
  ), true)
  assert.equal(sameStreamRun(
    { pendingId:'assistant-1', startedAtMs:10 },
    { pendingId:'assistant-2', startedAtMs:10 },
  ), false)
  assert.equal(sameStreamRun({ startedAtMs:10 }, { startedAtMs:10 }), true)
  assert.equal(sameStreamRun({}, {}), false)
})

test('terminal replay is never mistaken for the next loop round', () => {
  const currentRun = { pendingId:'assistant-1', startedAtMs:10 }
  assert.equal(decideStreamFollow({
    running:true,
    loop:{ enabled:true, status:'running' },
    currentRun,
    availableRun:{ pendingId:'assistant-1', startedAtMs:10 },
    terminal:true,
  }), 'wait')
  assert.equal(decideStreamFollow({
    running:true,
    loop:{ enabled:true, status:'running' },
    currentRun,
    availableRun:{ pendingId:'assistant-2', startedAtMs:20 },
    terminal:true,
  }), 'attach')
})

test('explicit run admission waits across the idle creation gap', () => {
  assert.equal(decideStreamFollow({
    running:false,
    currentRun:{ pendingId:'', startedAtMs:0 },
    availableRun:{ pendingId:'', startedAtMs:0 },
    awaitingRun:true,
  }), 'wait')
  assert.equal(decideStreamFollow({
    running:false,
    currentRun:{ pendingId:'assistant-1', startedAtMs:10 },
    awaitingRun:true,
  }), 'finish')
  assert.equal(decideStreamFollow({ running:false, awaitingRun:false }), 'finish')
})

test('loop waits across the no-run evaluation gap and finishes only after loop terminal state', () => {
  assert.equal(decideStreamFollow({
    running:false,
    loop:{ enabled:true, status:'evaluating' },
    terminal:true,
  }), 'wait')
  assert.equal(decideStreamFollow({
    running:false,
    loop:{ enabled:false, status:'completed' },
    terminal:true,
  }), 'finish')
  assert.equal(decideStreamFollow({ running:false, loop:null, terminal:true }), 'finish')
})

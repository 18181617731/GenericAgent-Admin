import test from 'node:test'
import assert from 'node:assert/strict'
import { getAskUserPayload, parseAskUserPayload, stripAskUserFence } from './askUserPayload.js'

test('strips three or more backtick fences used by verbose tool args', () => {
  const fenced = '````text\n{"question":"Continue?","candidates":["yes"]}\n````'
  assert.equal(stripAskUserFence(fenced), '{"question":"Continue?","candidates":["yes"]}')
  const parsed = parseAskUserPayload(fenced)
  assert.equal(parsed.question, 'Continue?')
  assert.deepEqual(parsed.candidates, ['yes'])
})

test('parses nested ask_user interrupt result payload', () => {
  const result = JSON.stringify({
    status: 'INTERRUPT',
    intent: 'HUMAN_INTERVENTION',
    data: { question: 'Pick one', candidates: ['A', 'B'] },
  })
  assert.deepEqual(parseAskUserPayload(result), {
    question: 'Pick one',
    candidates: ['A', 'B'],
    raw: result,
    structured: true,
  })
})

test('prefers structured result over compact args', () => {
  const ask = getAskUserPayload({
    args: '{"question":"short"}',
    result: '{"data":{"question":"full question","candidates":["x"]}}',
  })
  assert.equal(ask.question, 'full question')
  assert.deepEqual(ask.candidates, ['x'])
})

test('falls back without truncating escaped quotes and newline in question', () => {
  const raw = '[Result]\n{"question":"Need \\"quoted\\" detail?\\nSecond line","candidates":["Use \\"A\\"","Use B"]'
  const parsed = parseAskUserPayload(raw)
  assert.equal(parsed.question, 'Need "quoted" detail?\nSecond line')
  assert.deepEqual(parsed.candidates, ['Use "A"', 'Use B'])
})

test('extracts balanced JSON object from surrounding log text', () => {
  const raw = 'before {"question":"Brace in string { ok }","candidates":["one"]} after {not json}'
  const parsed = parseAskUserPayload(raw)
  assert.equal(parsed.question, 'Brace in string { ok }')
  assert.deepEqual(parsed.candidates, ['one'])
})


test('uses structured args when result is the plain waiting marker', () => {
  const args = JSON.stringify({ question: '\u8fd9\u662f\u4e00\u6761\u6d4b\u8bd5\u95ee\u9898\uff0c\u8bf7\u95ee\u4f60\u60f3\u8ba9\u6211\u5e2e\u4f60\u505a\u4ec0\u4e48\uff1f' })
  const ask = getAskUserPayload({
    args,
    result: 'Waiting for your answer ...',
  })
  assert.equal(ask.question, '\u8fd9\u662f\u4e00\u6761\u6d4b\u8bd5\u95ee\u9898\uff0c\u8bf7\u95ee\u4f60\u60f3\u8ba9\u6211\u5e2e\u4f60\u505a\u4ec0\u4e48\uff1f')
  assert.deepEqual(ask.candidates, [])
})

test('keeps candidates from args when result is the plain waiting marker', () => {
  const args = JSON.stringify({
    question: '\u4eca\u5929\u60f3\u559d\u70b9\u4ec0\u4e48\uff1f',
    candidates: ['\u5496\u5561', '\u8336', '\u679c\u6c41', '\u767d\u5f00\u6c34'],
  })
  const ask = getAskUserPayload({
    args,
    result: 'Waiting for your answer ...',
  })
  assert.equal(ask.question, '\u4eca\u5929\u60f3\u559d\u70b9\u4ec0\u4e48\uff1f')
  assert.deepEqual(ask.candidates, ['\u5496\u5561', '\u8336', '\u679c\u6c41', '\u767d\u5f00\u6c34'])
})

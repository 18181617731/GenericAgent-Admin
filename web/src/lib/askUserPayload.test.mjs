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

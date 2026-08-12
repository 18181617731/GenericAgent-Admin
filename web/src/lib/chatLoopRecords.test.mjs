import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { normalizeLoopRecords } from './chatLoopRecords.js'

test('normalizeLoopRecords bounds, sanitizes, and orders records newest first', () => {
  const records = Array.from({ length: 45 }, (_, index) => ({
    created_at_ms: 1000 + index,
    round: index,
    phase: index === 44 ? ' ERROR ' : 'checking',
    summary: `record ${index}`,
    prompt: index === 44 ? 'next action' : '',
  }))
  const view = normalizeLoopRecords({ records })
  assert.equal(view.length, 40)
  assert.equal(view[0].summary, 'record 44')
  assert.equal(view[0].phase, 'error')
  assert.equal(view[0].prompt, 'next action')
  assert.equal(view.at(-1).summary, 'record 5')
})

test('normalizeLoopRecords tolerates old and malformed session payloads', () => {
  assert.deepEqual(normalizeLoopRecords(null), [])
  assert.deepEqual(normalizeLoopRecords({}), [])
  assert.deepEqual(normalizeLoopRecords({ records: [null, 'bad', { summary: '   ' }] }), [])
  assert.deepEqual(normalizeLoopRecords({ records: [{ round: -3, phase: '', summary: ' ready ' }] }), [{
    key: '0-0', atMS: 0, round: 0, phase: 'activity', summary: 'ready', prompt: '',
  }])
})

test('Loop rail renders bounded observer records without raw controller output labels', () => {
  const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
  assert.match(source, /normalizeLoopRecords\(loopState\)/)
  assert.match(source, /oa-loop-records/)
  assert.match(source, /record\.summary/)
  assert.match(source, /record\.prompt/)
  assert.doesNotMatch(source, /chain[-_ ]of[-_ ]thought|raw controller output/i)
})

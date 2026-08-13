import test from 'node:test'
import assert from 'node:assert/strict'
import { buildLogRows, logLineLevel, splitLogMatch } from './logLines.js'

test('severity is read from the level conventions GA services emit', () => {
  assert.equal(logLineLevel('2026-08-13 10:00:00 ERROR failed to open socket'), 'error')
  assert.equal(logLineLevel('[WARNING] retrying in 5s'), 'warn')
  assert.equal(logLineLevel('CRITICAL: giving up'), 'error')
  assert.equal(logLineLevel('Traceback (most recent call last):'), 'error')
  assert.equal(logLineLevel('INFO worker ready'), '')
  // A level name has to stand alone, so ordinary prose is not painted red.
  assert.equal(logLineLevel('reporterror_count=0'), '')
  assert.equal(logLineLevel('downloading warnings.tar.gz'), '')
})

test('filtered rows keep the line numbers they had in the tail', () => {
  const lines = ['boot', 'ERROR disk full', 'retry', 'ERROR disk still full']
  const all = buildLogRows(lines)
  assert.deepEqual(all.map(row => row.number), [1, 2, 3, 4])

  const errors = buildLogRows(lines, 'error')
  assert.deepEqual(errors.map(row => row.number), [2, 4])
  assert.deepEqual(errors.map(row => row.level), ['error', 'error'])
})

test('an all-whitespace filter is not a filter', () => {
  assert.equal(buildLogRows(['a', 'b'], '   ').length, 2)
  assert.deepEqual(splitLogMatch('a', '   '), [{ text: 'a', match: false }])
})

test('matches are split into inert segments instead of injected markup', () => {
  assert.deepEqual(splitLogMatch('ab-AB-ab', 'ab'), [
    { text: 'ab', match: true },
    { text: '-', match: false },
    { text: 'AB', match: true },
    { text: '-', match: false },
    { text: 'ab', match: true },
  ])
  assert.deepEqual(splitLogMatch('<b>hi</b>', 'hi'), [
    { text: '<b>', match: false },
    { text: 'hi', match: true },
    { text: '</b>', match: false },
  ])
  assert.deepEqual(splitLogMatch('plain', 'zzz'), [{ text: 'plain', match: false }])
})

test('missing and non-string input is tolerated', () => {
  assert.deepEqual(buildLogRows(null), [])
  assert.deepEqual(buildLogRows([undefined, 7]).map(row => row.text), ['', '7'])
  assert.equal(logLineLevel(undefined), '')
})

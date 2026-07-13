import test from 'node:test'
import assert from 'node:assert/strict'

import { REASONING_EFFORT_LEVELS, normalizeReasoningEffort } from './reasoningEffort.js'

const OFFICIAL_LEVELS = ['off', 'none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max']

test('reasoning effort exposes every official GA level in order', () => {
  assert.deepEqual([...REASONING_EFFORT_LEVELS], OFFICIAL_LEVELS)
})

test('reasoning effort normalization preserves every official GA level', () => {
  for (const level of OFFICIAL_LEVELS) {
    assert.equal(normalizeReasoningEffort(` ${level.toUpperCase()} `), level)
  }
  assert.equal(normalizeReasoningEffort('unsupported'), 'off')
  assert.equal(normalizeReasoningEffort(''), 'off')
})

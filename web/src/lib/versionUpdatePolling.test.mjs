import test from 'node:test'
import assert from 'node:assert/strict'

import { shouldAdoptStatusCheck } from './versionUpdatePolling.js'

test('adopts the check snapshot from an active update transaction', () => {
  assert.equal(shouldAdoptStatusCheck({ running: true, check: { latest: { tag_name: 'v0.2.13' } } }), true)
})

test('does not let a completed historical transaction overwrite a fresh version check', () => {
  assert.equal(shouldAdoptStatusCheck({ running: false, stage: 'done', check: { latest: { tag_name: 'v0.2.10' } } }), false)
})

test('does not adopt an empty active transaction snapshot', () => {
  assert.equal(shouldAdoptStatusCheck({ running: true }), false)
})

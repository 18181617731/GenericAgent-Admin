import test from 'node:test'
import assert from 'node:assert/strict'

import { cacheReadTokens } from './chatUsage.js'

test('cacheReadTokens falls back to legacy cached_tokens when canonical value is zero', () => {
  assert.equal(cacheReadTokens({
    cache_read_tokens: 0,
    cached_tokens: 821500,
  }), 821500)
})

test('cacheReadTokens prefers a positive canonical value without double counting the alias', () => {
  assert.equal(cacheReadTokens({
    cache_read_tokens: 420,
    cached_tokens: 821500,
  }), 420)
})

test('cacheReadTokens handles legacy-only and empty usage objects', () => {
  assert.equal(cacheReadTokens({ cached_tokens: 125 }), 125)
  assert.equal(cacheReadTokens({}), 0)
  assert.equal(cacheReadTokens(null), 0)
})

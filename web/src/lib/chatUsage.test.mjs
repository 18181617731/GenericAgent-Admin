import test from 'node:test'
import assert from 'node:assert/strict'

import { cacheHitPercent, cacheReadTokens, measuredOutputRate } from './chatUsage.js'

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

test('cacheHitPercent uses only cacheable tokens for modern buckets', () => {
  assert.equal(cacheHitPercent([
    { input_tokens: 100, cache_creation_tokens: 40, cache_read_tokens: 160 },
  ]), 53)
})

test('cacheHitPercent does not double count legacy cached tokens', () => {
  assert.equal(cacheHitPercent([
    { input_tokens: 100, cached_tokens: 80 },
  ]), 80)
})

test('cacheHitPercent supports mixed legacy and modern usage', () => {
  assert.equal(cacheHitPercent([
    { input_tokens: 100, cached_tokens: 80 },
    { input_tokens: 50, cache_creation_tokens: 50, cache_read_tokens: 100 },
  ]), 60)
})

test('measuredOutputRate divides only measured outputs by measured generation time', () => {
  assert.equal(measuredOutputRate([
    { output_tokens: 100, generation_ms: 2000 },
    { output_tokens: 900 },
    { output_tokens: 50, generation_ms: 500 },
  ]), 60)
})

test('measuredOutputRate avoids false precision when no generation interval exists', () => {
  assert.equal(measuredOutputRate([{ output_tokens: 100 }, { output_tokens: 20, generation_ms: 0 }]), 0)
  assert.equal(measuredOutputRate(null), 0)
})

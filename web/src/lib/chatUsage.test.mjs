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

test('cacheHitPercent calculates cache_read / (output + cache_read)', () => {
  // cache_read=160, output=100 → 160 / (100 + 160) = 160 / 260 ≈ 62%
  assert.equal(cacheHitPercent([
    { cache_read_tokens: 160, output_tokens: 100 },
  ]), 62)
})

test('cacheHitPercent uses legacy cached_tokens with output denominator', () => {
  // For legacy APIs, cached_tokens describes input caching, but we still
  // calculate rate as cached / (output + cached) for consistency
  // cached=80, output=20 → 80 / (20 + 80) = 80 / 100 = 80%
  assert.equal(cacheHitPercent([
    { cached_tokens: 80, output_tokens: 20 },
  ]), 80)
})

test('cacheHitPercent supports mixed legacy and modern usage', () => {
  // Legacy: cached=80, output=20 → 80
  // Modern: cache_read=100, output=50 → 100
  // Total: (80 + 100) / (20 + 80 + 50 + 100) = 180 / 250 = 72%
  assert.equal(cacheHitPercent([
    { cached_tokens: 80, output_tokens: 20 },
    { cache_read_tokens: 100, output_tokens: 50 },
  ]), 72)
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

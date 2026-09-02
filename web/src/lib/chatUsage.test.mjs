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

test('cacheHitPercent reproduces the real OpenAI session with prompt input as denominator', () => {
  const usages = [
    { input_tokens: 28347, cache_read_tokens: 28800, output_tokens: 500, input_tokens_include_cache_read: 1 },
    { input_tokens: 30000, cache_read_tokens: 28800, output_tokens: 500, input_tokens_include_cache_read: 1 },
    { input_tokens: 30000, cache_read_tokens: 28800, output_tokens: 500, input_tokens_include_cache_read: 1 },
    { input_tokens: 30000, cache_read_tokens: 28800, output_tokens: 500, input_tokens_include_cache_read: 1 },
    { input_tokens: 24999, cache_read_tokens: 0, output_tokens: 523, input_tokens_include_cache_read: 1 },
  ]
  // input=143346, cache read=115200, output=2523. Output is not part of the rate.
  assert.equal(cacheHitPercent(usages), 80)
})

test('cacheHitPercent adds disjoint Claude input, creation, and read buckets', () => {
  assert.equal(cacheHitPercent([
    { input_tokens: 100, cache_creation_tokens: 40, cache_read_tokens: 160, input_tokens_include_cache_read: 0 },
  ]), 53)
})

test('cacheHitPercent includes zero-hit turns in the prompt denominator', () => {
  assert.equal(cacheHitPercent([
    { input_tokens: 100, cache_read_tokens: 80, input_tokens_include_cache_read: 1 },
    { input_tokens: 100, cache_read_tokens: 0, input_tokens_include_cache_read: 1 },
  ]), 40)
})

test('cacheHitPercent recovers old OpenAI sessions and legacy aliases', () => {
  assert.equal(cacheHitPercent([
    { input_tokens: 143346, cache_read_tokens: 115200, output_tokens: 2523 },
  ]), 80)
  assert.equal(cacheHitPercent([
    { input_tokens: 100, cached_tokens: 80, output_tokens: 20 },
  ]), 80)
})

test('cacheHitPercent aggregates mixed provider turns using total prompt input', () => {
  assert.equal(cacheHitPercent([
    { input_tokens: 100, cache_read_tokens: 80, input_tokens_include_cache_read: 1 },
    { input_tokens: 20, cache_read_tokens: 80, input_tokens_include_cache_read: 0 },
  ]), 80)
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

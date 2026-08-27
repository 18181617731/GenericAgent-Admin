import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')

test('chat omits non-error message notices while retaining error feedback', () => {
  assert.doesNotMatch(chatSource, /\{notice\s*&&/)
  assert.doesNotMatch(chatSource, /<span>\{notice\}<\/span>/)
  assert.match(chatSource, /\{err\s*&&\s*<div className="oa-banner error">/)
  assert.match(chatSource, /<span>\{err\}<\/span>/)
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')

test('chat keeps feedback in the local compact surface and marks errors accessibly', () => {
  assert.doesNotMatch(chatSource, /\{notice\s*&&/)
  assert.doesNotMatch(chatSource, /<span>\{notice\}<\/span>/)
  assert.doesNotMatch(chatSource, /<div className="oa-banner error">/)
  assert.match(chatSource, /oa-chat-feedback/)
  assert.match(chatSource, /role=\{err \? 'alert' : 'status'\}/)
  assert.match(chatSource, /\(err \|\| notice\)/)
})

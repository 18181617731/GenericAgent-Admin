import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  CHAT_SESSION_SELECTION_STORAGE_KEY,
  chooseChatSessionID,
  loadSelectedChatSessionID,
  persistSelectedChatSessionID,
} from './chatSessionSelection.js'

function memoryStorage(initial = {}) {
  const values = new Map(Object.entries(initial))
  return {
    getItem(key) { return values.has(key) ? values.get(key) : null },
    setItem(key, value) { values.set(key, String(value)) },
    removeItem(key) { values.delete(key) },
    snapshot() { return Object.fromEntries(values) },
  }
}

test('selected chat sessions are remembered independently per instance', () => {
  const storage = memoryStorage()
  persistSelectedChatSessionID('', 'default-session', storage)
  persistSelectedChatSessionID('alpha', 'alpha-session', storage)

  assert.equal(loadSelectedChatSessionID('', storage), 'default-session')
  assert.equal(loadSelectedChatSessionID('alpha', storage), 'alpha-session')
  assert.equal(loadSelectedChatSessionID('beta', storage), '')

  persistSelectedChatSessionID('alpha', '', storage)
  assert.equal(loadSelectedChatSessionID('alpha', storage), '')
  assert.equal(loadSelectedChatSessionID('', storage), 'default-session')
})

test('selection storage tolerates malformed and unavailable storage', () => {
  const malformed = memoryStorage({ [CHAT_SESSION_SELECTION_STORAGE_KEY]: '{bad json' })
  assert.equal(loadSelectedChatSessionID('', malformed), '')
  assert.doesNotThrow(() => persistSelectedChatSessionID('', 'session-1', {
    getItem() { throw new Error('blocked') },
    setItem() { throw new Error('blocked') },
    removeItem() { throw new Error('blocked') },
  }))
})

test('session choice requires IDs to exist and follows preference order', () => {
  const sessions = [{ id: 'first' }, { id: 'preferred' }, { id: 'restored' }]
  assert.equal(chooseChatSessionID(sessions, 'preferred', 'restored'), 'preferred')
  assert.equal(chooseChatSessionID(sessions, 'missing', 'restored'), 'restored')
  assert.equal(chooseChatSessionID(sessions, 'missing', 'stale'), 'first')
  assert.equal(chooseChatSessionID([], 'preferred', 'restored'), '')
})

test('ChatApp persists successful opens and restores a valid selection on mount', () => {
  const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
  assert.match(source, /persistSelectedChatSessionID\(chatInstanceRef\.current, d\.id\)/)
  assert.match(source, /const restored = loadSelectedChatSessionID\(chatInstanceRef\.current\)/)
  assert.match(source, /const next = chooseChatSessionID\(list, prefer, restored\)/)
  assert.match(source, /persistSelectedChatSessionID\(chatInstanceRef\.current, ''\)/)
  assert.match(source, /loadSessions\('', \{ open:true \}\)/)
})

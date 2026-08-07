import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../pages/ChatPage.jsx', import.meta.url), 'utf8')

test('primes iOS completion audio before a new chat session awaits the API', () => {
  const sendStart = source.indexOf('const send = async () =>')
  const prime = source.indexOf('primeChatCompletionTone()', sendStart)
  const createSession = source.indexOf("await api('/api/chat/session/new'", sendStart)
  assert.ok(sendStart >= 0)
  assert.ok(prime > sendStart)
  assert.ok(createSession > prime)
})

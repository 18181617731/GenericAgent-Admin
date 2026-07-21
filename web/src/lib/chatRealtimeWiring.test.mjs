import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')

test('routes btw independently before the busy queue', () => {
  assert.match(source, /api\(`\/api\/chat\/btw\/\$\{sessionId\}`/)
  assert.ok(source.indexOf('if (isBTWCommand(text)') < source.indexOf('if (busy) {\n      enqueueMessage(item)'))
})

test('follows interrupted streams with an event cursor', () => {
  assert.match(source, /chat\/stream\/\$\{sessionId\}\?from=\$\{cursor\}/)
  assert.match(source, /cursor \+= eventCount/)
  assert.match(source, /chatStreamOutcome/)
  assert.match(source, /state = await api\(`\/api\/chat\/state\/\$\{sessionId\}`/)
  assert.match(source, /followChatStream\(res, pendingId/)
  assert.match(source, /followChatStream\(res, pending\.id/)
})

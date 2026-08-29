import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')

test('routes btw independently before the busy queue', () => {
  assert.match(source, /chatApi\(`\/api\/chat\/btw\/\$\{sessionId\}`/)
  const btwBranch = source.indexOf('if (isBTWCommand(text)')
  const busyBranch = source.indexOf('if (busy || activeRunRef.current)', btwBranch)
  const enqueueCall = source.indexOf('enqueueMessage(item)', busyBranch)
  assert.ok(btwBranch >= 0)
  assert.ok(busyBranch > btwBranch)
  assert.ok(enqueueCall > busyBranch)
})

test('follows interrupted streams with an event cursor', () => {
  assert.match(source, /chat\/stream\/\$\{sessionId\}\?from=\$\{cursor\}/)
  assert.match(source, /cursor \+= eventCount/)
  assert.match(source, /chatStreamOutcome/)
  assert.match(source, /state = await chatApi\(`\/api\/chat\/state\/\$\{sessionId\}`/)
  assert.match(source, /followChatStream\(res, pendingId/)
  assert.match(source, /followChatStream\(res, pending\.id/)
})

test('primes completion audio on send and keeps queue execution backend-authoritative', () => {
  const primeIndex = source.indexOf('primeChatCompletionTone()')
  const queueIndex = source.indexOf("requestQueue('enqueue'")
  const notifyIndex = source.indexOf('if (streamCompleted) publishNotification(', primeIndex)
  assert.ok(primeIndex >= 0)
  assert.ok(queueIndex >= 0)
  assert.ok(notifyIndex > primeIndex)
  assert.match(source, /Queue execution is backend-authoritative/)
  assert.doesNotMatch(source, /const next = popQueued\(\)/)
  assert.match(source, /buildChatNotification\(\{ session: sessionForNotification, sessionId: id, prompt: notificationPrompt/)
  assert.doesNotMatch(source, /message:\s*`会话 \$\{id\} 已完成回复。`/)
})

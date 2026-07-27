import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')

test('routes btw independently before the busy queue', () => {
  assert.match(source, /api\(`\/api\/chat\/btw\/\$\{sessionId\}`/)
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
  assert.match(source, /state = await api\(`\/api\/chat\/state\/\$\{sessionId\}`/)
  assert.match(source, /followChatStream\(res, pendingId/)
  assert.match(source, /followChatStream\(res, pending\.id/)
})

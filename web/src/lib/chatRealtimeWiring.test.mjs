import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')

test('routes btw independently before the busy queue', () => {
  assert.match(source, /api\(`\/api\/chat\/btw\/\$\{sessionId\}`/)
  const btwBranch = source.indexOf('if (isBTWCommand(text)')
  const enqueueCall = source.indexOf('enqueueMessage(item)', btwBranch)
  const busyBranch = source.lastIndexOf('if (busy)', enqueueCall)
  assert.ok(btwBranch >= 0)
  assert.ok(enqueueCall > btwBranch)
  assert.ok(busyBranch > btwBranch && busyBranch < enqueueCall)
})

test('follows interrupted streams with an event cursor', () => {
  assert.match(source, /chat\/stream\/\$\{sessionId\}\?from=\$\{cursor\}/)
  assert.match(source, /cursor \+= eventCount/)
  assert.match(source, /chatStreamOutcome/)
  assert.match(source, /state = await api\(`\/api\/chat\/state\/\$\{sessionId\}`/)
  assert.match(source, /followChatStream\(res, pendingId/)
  assert.match(source, /followChatStream\(res, pending\.id/)
})

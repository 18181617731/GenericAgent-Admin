import assert from 'node:assert/strict'
import test from 'node:test'
import { pollGeneratedChatTitle, shouldPollGeneratedTitle } from './chatTitlePolling.js'

test('pollGeneratedChatTitle stops when the generated title arrives', async () => {
  const snapshots = [
    [{ id:'s1', title:'first message', title_source:'temporary' }],
    [{ id:'s1', title:'AI conversation titles', title_source:'generated' }],
  ]
  let calls = 0
  const result = await pollGeneratedChatTitle({
    sessionId:'s1',
    wait:async () => {},
    loadSessions:async () => snapshots[Math.min(calls++, snapshots.length - 1)],
  })

  assert.equal(calls, 2)
  assert.deepEqual(result, snapshots[1][0])
  assert.equal(shouldPollGeneratedTitle(result), false)
})

test('pollGeneratedChatTitle stops when the user leaves the session', async () => {
  let active = true
  let calls = 0
  const result = await pollGeneratedChatTitle({
    sessionId:'s1',
    isActive:() => active,
    wait:async () => { active = false },
    loadSessions:async () => {
      calls += 1
      return []
    },
  })

  assert.equal(result, null)
  assert.equal(calls, 0)
})

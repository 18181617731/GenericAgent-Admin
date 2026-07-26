import test from 'node:test'
import assert from 'node:assert/strict'
import { deleteChatSessions, generateChatSessionTitles, normalizeSessionIds } from './chatSessionManagement.js'

test('normalizeSessionIds keeps unique non-empty session ids in order', () => {
  assert.deepEqual(normalizeSessionIds(['alpha', '', 'alpha', null, 'beta', '  ', 'gamma']), ['alpha', 'beta', 'gamma'])
})

test('deleteChatSessions deletes every selected session and reports partial failures', async () => {
  const started = []
  const release = new Map()
  const deleteOne = (id) => new Promise((resolve, reject) => {
    started.push(id)
    release.set(id, id === 'beta' ? () => reject(new Error('locked')) : resolve)
  })

  const pending = deleteChatSessions(['alpha', 'beta', 'alpha', 'gamma'], deleteOne)
  await Promise.resolve()
  assert.deepEqual(started, ['alpha', 'beta', 'gamma'], 'deletes should start concurrently')

  release.get('alpha')()
  release.get('beta')()
  release.get('gamma')()

  const result = await pending
  assert.deepEqual(result.deletedIds, ['alpha', 'gamma'])
  assert.deepEqual(result.failedIds, ['beta'])
  assert.equal(result.failures[0].id, 'beta')
  assert.match(result.failures[0].error.message, /locked/)
})

test('deleteChatSessions is a no-op for an empty selection', async () => {
  let calls = 0
  const result = await deleteChatSessions([], async () => { calls += 1 })
  assert.equal(calls, 0)
  assert.deepEqual(result, { deletedIds: [], failedIds: [], failures: [] })
})

test('generateChatSessionTitles limits concurrency and reports partial failures', async () => {
  let active = 0
  let maxActive = 0
  const releases = []
  const generateOne = id => new Promise((resolve, reject) => {
    active += 1
    maxActive = Math.max(maxActive, active)
    releases.push(() => {
      active -= 1
      if (id === 'beta') reject(new Error('model unavailable'))
      else resolve({ id, title: `title-${id}` })
    })
  })

  const pending = generateChatSessionTitles(['alpha', 'beta', 'gamma'], generateOne, 2)
  await Promise.resolve()
  assert.equal(maxActive, 2)
  releases.shift()()
  await Promise.resolve()
  releases.shift()()
  await Promise.resolve()
  releases.shift()()

  const result = await pending
  assert.deepEqual(result.generatedIds.sort(), ['alpha', 'gamma'])
  assert.deepEqual(result.failedIds, ['beta'])
  assert.equal(result.generated.find(item => item.id === 'gamma').session.title, 'title-gamma')
  assert.match(result.failures[0].error.message, /unavailable/)
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { createChatSessionListReader } from './chatSessionListReader.js'

const response = (data, etag = '"v1"', status = 200) => new Response(status === 304 ? null : JSON.stringify(data), { status, headers: { ETag: etag } })
const snapshot = { sessions: [{ id: 'a', running: false }], projects: [] }

test('overlapping background reads share one network request but independent views', async () => {
  const reader = createChatSessionListReader()
  let release
  const gate = new Promise(resolve => { release = resolve })
  let calls = 0
  const fetcher = async () => { calls++; await gate; return response(snapshot) }
  const a = reader.load('/sessions?instance=one', { fetcher })
  const b = reader.load('/sessions?instance=one', { fetcher })
  assert.equal(calls, 1)
  release()
  const [first, second] = await Promise.all([a, b])
  assert.deepEqual(first, second)
  first.sessions[0].running = true
  assert.equal(second.sessions[0].running, false)
  const cached = await reader.load('/sessions?instance=one', { fetcher: async (url, options) => {
    assert.equal(options.headers['If-None-Match'], '"v1"')
    return response(null, '"v1"', 304)
  } })
  assert.equal(cached.sessions[0].running, false)
})

test('post-mutation fresh read supersedes an older in-flight request', async () => {
  const reader = createChatSessionListReader()
  let release
  const gate = new Promise(resolve => { release = resolve })
  const old = reader.load('/sessions', { fetcher: async () => { await gate; return response(snapshot) } })
  const rejected = assert.rejects(old, { name: 'AbortError' })
  const updated = { sessions: [{ id: 'b', running: true }] }
  assert.deepEqual(await reader.load('/sessions', { fresh: true, fetcher: async () => response(updated, '"v2"') }), updated)
  release()
  await rejected
  assert.deepEqual(await reader.load('/sessions', { fetcher: async (url, options) => {
    assert.equal(options.headers['If-None-Match'], '"v2"')
    return response(null, '"v2"', 304)
  } }), updated)
})

test('instance URLs are isolated and clear prevents late responses from restoring snapshots', async () => {
  const reader = createChatSessionListReader()
  await reader.load('/sessions?instance=one', { fetcher: async () => response(snapshot) })
  await reader.load('/sessions?instance=two', { fetcher: async (url, options) => {
    assert.equal(options.headers['If-None-Match'], undefined)
    return response(snapshot, '"v2"')
  } })
  let release
  const gate = new Promise(resolve => { release = resolve })
  const old = reader.load('/sessions?instance=one', { fetcher: async () => { await gate; return response(null, '"v1"', 304) } })
  const rejected = assert.rejects(old, { name: 'AbortError' })
  reader.clear()
  release()
  await rejected
  await reader.load('/sessions?instance=one', { fetcher: async (url, options) => {
    assert.equal(options.headers['If-None-Match'], undefined)
    return response(snapshot)
  } })
})

test('a failed request does not poison later refreshes', async () => {
  const reader = createChatSessionListReader()
  await assert.rejects(reader.load('/sessions', { fetcher: async () => { throw new Error('offline') } }), /offline/)
  assert.deepEqual(await reader.load('/sessions', { fetcher: async () => response(snapshot) }), snapshot)
})

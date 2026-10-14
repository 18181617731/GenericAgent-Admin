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
})

test('sync snapshots merge additions, removals, live state and metadata in stable order', async () => {
  const reader = createChatSessionListReader()
  const initial = { sessions: [{ id: 'b', updated_at: 2 }, { id: 'a', updated_at: 1 }], projects: ['old'], pinned_projects: [] }
  await reader.load('/sessions?instance=one', { fetcher: async url => {
    assert.equal(url, '/sessions?instance=one&sync=1')
    return response({ ...initial, reset: true, revision: 'r1' })
  } })
  const merged = await reader.load('/sessions?instance=one', { fetcher: async (url, options) => {
    assert.equal(url, '/sessions?instance=one&sync=1&since=r1')
    assert.equal(options.headers['If-None-Match'], undefined)
    return response({ reset: false, base_revision: 'r1', revision: 'r2', removed_session_ids: ['b'], session_upserts: [{ id: 'a', updated_at: 1, running: true, unread: true }, { id: 'c', updated_at: 1 }], projects: ['new'] })
  } })
  assert.deepEqual(merged, { ...initial, sessions: [{ id: 'a', updated_at: 1, running: true, unread: true }, { id: 'c', updated_at: 1 }], projects: ['new'] })
  merged.sessions[0].running = false
  const unchanged = await reader.load('/sessions?instance=one', { fetcher: async url => {
    assert.match(url, /since=r2$/)
    return new Response(null, { status: 204 })
  } })
  assert.equal(unchanged.sessions[0].running, true)
})

test('mismatched delta retries without cursor and expired cursor accepts a reset', async () => {
  const reader = createChatSessionListReader()
  await reader.load('/sessions', { fetcher: async () => response({ ...snapshot, reset: true, revision: 'r1' }) })
  const urls = []
  const data = await reader.load('/sessions', { fetcher: async url => {
    urls.push(url)
    return urls.length === 1
      ? response({ reset: false, base_revision: 'wrong', revision: 'r2', session_upserts: [] })
      : response({ sessions: [], projects: ['reset'], reset: true, revision: 'r3' })
  } })
  assert.deepEqual(urls, ['/sessions?sync=1&since=r1', '/sessions?sync=1'])
  assert.deepEqual(data, { sessions: [], projects: ['reset'] })
  assert.deepEqual(await reader.load('/sessions', { fetcher: async () => response({ ...snapshot, reset: true, revision: 'r4' }) }), snapshot)
})

test('eviction never sends a cursor whose snapshot is missing', async () => {
  const reader = createChatSessionListReader({ maxEntries: 1 })
  for (const url of ['/one', '/two', '/one']) {
    await reader.load(url, { fetcher: async requested => {
      assert.equal(requested, `${url}?sync=1`)
      return response({ ...snapshot, reset: true, revision: url })
    } })
  }
  const tiny = createChatSessionListReader({ maxBytes: 1 })
  for (let i = 0; i < 2; i++) await tiny.load('/one', { fetcher: async url => {
    assert.equal(url, '/one?sync=1')
    return response({ ...snapshot, reset: true, revision: 'large' })
  } })
})

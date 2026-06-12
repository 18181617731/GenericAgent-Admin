import test from 'node:test'
import assert from 'node:assert/strict'
import { buildWorkerSetting, isLikelyHttpUrl, normalizeBBSConnection, REQUIRED_BBS_ENDPOINTS, validateBBSConnectionInput, validateBBSReadmeContract } from './bbsContract.js'

test('normalizeBBSConnection prefers builtin admin base when builtin mode', () => {
  const conn = normalizeBBSConnection({
    status: { enabled: true, mode: 'builtin', base_url: 'http://admin', builtin_base_url: 'http://builtin', posts: 7 },
    config: { mode: 'builtin', board_key: 'team-key' },
  })
  assert.equal(conn.activeBase, 'http://builtin')
  assert.equal(conn.boardKey, 'team-key')
  assert.equal(conn.postCount, 7)
  assert.equal(conn.enabled, true)
})

test('normalizeBBSConnection uses external base only in external mode', () => {
  const conn = normalizeBBSConnection({
    status: { enabled: true, base_url: 'http://admin' },
    config: { mode: 'external', base_url: 'http://remote', board_key: 'remote-key' },
  })
  assert.equal(conn.mode, 'external')
  assert.equal(conn.activeBase, 'http://remote')
  assert.equal(conn.externalBase, 'http://remote')
})

test('buildWorkerSetting emits worker-compatible JSON', () => {
  const conn = normalizeBBSConnection({ config: { mode: 'external', base_url: 'http://bbs', board_key: 'k' } })
  assert.deepEqual(JSON.parse(buildWorkerSetting(conn, 'worker-a')), { base_url: 'http://bbs', board_key: 'k', name: 'worker-a' })
})

test('validateBBSReadmeContract requires all worker endpoints and GA worker hint', () => {
  const good = `GA worker:\n${REQUIRED_BBS_ENDPOINTS.join('\n')}`
  assert.equal(validateBBSReadmeContract(good), true)
  assert.equal(validateBBSReadmeContract(good.replace('POST /reply?key=BOARD_KEY', '')), false)
  assert.equal(validateBBSReadmeContract(REQUIRED_BBS_ENDPOINTS.join('\n')), false)
})


test('validateBBSConnectionInput rejects blank or invalid external board config', () => {
  assert.deepEqual(validateBBSConnectionInput({ mode: 'external', base_url: '', board_key: 'k' }), { ok: false, errors: ['External BBS needs a base URL.'] })
  assert.equal(validateBBSConnectionInput({ mode: 'external', base_url: 'not-a-url', board_key: 'k' }).ok, false)
  assert.equal(validateBBSConnectionInput({ mode: 'external', base_url: 'https://bbs.local', board_key: 'k' }).ok, true)
  assert.equal(validateBBSConnectionInput({ mode: 'builtin', board_key: '' }).ok, false)
})

test('normalizeBBSConnection marks invalid external config not ready without stale success', () => {
  const conn = normalizeBBSConnection({
    status: { enabled: true, mode: 'external', base_url: 'http://previous', posts: 3 },
    config: { mode: 'external', base_url: 'ftp://bad', board_key: 'k' },
  })
  assert.equal(conn.enabled, false)
  assert.match(conn.error, /http\(s\)/)
  assert.equal(isLikelyHttpUrl('http://127.0.0.1:8787'), true)
  assert.equal(isLikelyHttpUrl(''), false)
})

test('normalizeBBSConnection disables error states and drops stale post counts', () => {
  const conn = normalizeBBSConnection({
    status: { enabled: true, mode: 'builtin', base_url: 'http://stale', error: 'bbs unavailable', posts: '12' },
    config: { mode: 'builtin', board_key: 'team-key' },
  })

  assert.equal(conn.enabled, false)
  assert.equal(conn.error, 'bbs unavailable')
  assert.equal(conn.postCount, 0)
  assert.equal(conn.activeBase, 'http://stale')
})

test('normalizeBBSConnection rejects missing external board key even with stale status success', () => {
  const conn = normalizeBBSConnection({
    status: { enabled: true, mode: 'external', base_url: 'http://previous', posts: 4 },
    config: { mode: 'external', base_url: 'http://remote', board_key: '' },
  })

  assert.equal(conn.enabled, false)
  assert.equal(conn.error, 'Board key is required.')
  assert.deepEqual(conn.inputErrors, ['Board key is required.'])
})

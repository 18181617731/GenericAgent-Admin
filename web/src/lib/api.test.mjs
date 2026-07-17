import test from 'node:test'
import assert from 'node:assert/strict'
import { apiHeaders, parseApiResponse, readableApiError } from './api.js'

test('apiHeaders adds JSON and dangerous confirm header', () => {
  assert.deepEqual(apiHeaders({ dangerous: true }), { 'X-GA-Confirm': 'dangerous', 'Content-Type': 'application/json' })
})

test('apiHeaders preserves explicit content-type', () => {
  assert.equal(apiHeaders({ headers: { 'content-type': 'text/plain' } })['Content-Type'], undefined)
})

test('parseApiResponse prefers structured error detail', async () => {
  const res = new Response(JSON.stringify({ detail: 'bad things' }), { status: 400, statusText: 'Bad Request' })
  await assert.rejects(() => parseApiResponse(res, '/x'), /bad things/)
})

test('readableApiError turns network failures into actionable feedback', () => {
  const error = readableApiError(new TypeError('Failed to fetch'), '/api/health')
  assert.match(error.message, /无法连接 GA Admin 服务/)
  assert.match(error.message, /\/api\/health/)
})

test('readableApiError preserves backend and abort errors', () => {
  const backend = new Error('permission denied')
  const aborted = Object.assign(new Error('aborted'), { name: 'AbortError' })
  assert.equal(readableApiError(backend, '/api/files'), backend)
  assert.equal(readableApiError(aborted), aborted)
})

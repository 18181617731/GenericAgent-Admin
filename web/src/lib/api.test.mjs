import test from 'node:test'
import assert from 'node:assert/strict'
import { apiHeaders, parseApiResponse } from './api.js'

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

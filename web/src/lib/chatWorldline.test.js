import test from 'node:test'
import assert from 'node:assert/strict'
import { applyWorldlineResponse, classifyWorldline, normalizeWorldline, worldlineErrorState, worldlineVersionForMessage } from './chatWorldline.js'

const tree = {
  available:true, head:'n2', current_path:['n1','n2','ghost'],
  nodes:[
    { id:'n1', parent_id:null, children:['n2'], depth:0, title:'Root' },
    { id:'n2', parent_id:'n1', children:[], depth:1, title:'Branch' },
  ],
  message_versions:{ u1:{ user_message_id:'u1', assistant_message_id:'a1', node_id:'n2', index:2, total:3, previous_node_id:'n0', next_node_id:'n3' } },
  assistant_message_ids:{ u1:'a1' },
}

test('normalizes the typed tree and removes impossible current-path entries', () => {
  const value = normalizeWorldline(tree)
  assert.deepEqual(value.current_path, ['n1','n2'])
  assert.equal(classifyWorldline(value), 'ready')
})

test('resolves one version group from either half of a rendered turn', () => {
  assert.equal(worldlineVersionForMessage(tree, 'u1').node_id, 'n2')
  assert.equal(worldlineVersionForMessage(tree, 'a1').node_id, 'n2')
  assert.equal(worldlineVersionForMessage(tree, 'missing'), null)
})

test('classifies empty, unavailable, and degraded responses explicitly', () => {
  assert.equal(classifyWorldline({ available:true, nodes:[] }), 'empty')
  assert.equal(classifyWorldline({ available:false }), 'unavailable')
  assert.equal(classifyWorldline({ available:false, degraded_reason:'sidecar unavailable' }), 'degraded')
})

test('applies typed switch/load envelopes and preserves stale data on refresh errors', () => {
  const ready = applyWorldlineResponse(null, { worldline:tree }, 'sid-1')
  assert.equal(ready.status, 'ready')
  assert.equal(ready.data.head, 'n2')
  const stale = worldlineErrorState(ready, new Error('offline'), 'sid-1')
  assert.equal(stale.status, 'stale-error')
  assert.equal(stale.data.head, 'n2')
})

test('caps oversized trees and marks the normalized projection as truncated', () => {
  const oversized = {
    ...tree,
    nodes: Array.from({ length: 505 }, (_, index) => ({ id:`n${index}`, parent_id:null, children:[] })),
    current_path:['n0', 'n504'],
  }
  const value = normalizeWorldline(oversized)
  assert.equal(value.nodes.length, 500)
  assert.equal(value.truncated, true)
  assert.deepEqual(value.current_path, ['n0'])
})

test('rejects unknown schema atomically and preserves the prior tree', () => {
  const ready = applyWorldlineResponse(null, { worldline:{ ...tree, schema_version:1 } }, 'sid-1')
  const incompatible = applyWorldlineResponse(ready, { worldline:{ ...tree, schema_version:99, head:'other' } }, 'sid-1')
  assert.equal(incompatible.status, 'stale-error')
  assert.match(incompatible.error, /schema version: 99/)
  assert.equal(incompatible.data.head, 'n2')
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { buildWorldlineEdges, buildWorldlineRows, worldlineMaxLevel, messageVersionInfo, worldlineNodeTitle } from './worldlineTree.js'

const N = (id, parent, ordinal = 0, extra = {}) => ({ id, parent_id: parent, ordinal, ...extra })

test('a single continuous chain stays on one rail level (no per-parent indent)', () => {
  const nodes = [N('a', null, 0), N('b', 'a', 0), N('c', 'b', 0), N('d', 'c', 0)]
  const rows = buildWorldlineRows(nodes, ['a', 'b', 'c', 'd'], 'd')
  assert.deepEqual(rows.map(r => r.node.id), ['a', 'b', 'c', 'd'])
  assert.deepEqual(rows.map(r => r.level), [0, 0, 0, 0])
  assert.equal(rows[3].isCurrent, true)
  assert.equal(worldlineMaxLevel(rows), 0)
})

test('a fork adds exactly one level for the side branch, current path stays primary', () => {
  // a - b - c (current path)  with side branch b - x - y
  const nodes = [N('a', null, 0), N('b', 'a', 1), N('c', 'b', 2), N('x', 'b', 3), N('y', 'x', 4)]
  const rows = buildWorldlineRows(nodes, ['a', 'b', 'c'], 'c')
  const byId = Object.fromEntries(rows.map(r => [r.node.id, r]))
  assert.equal(byId.a.level, 0)
  assert.equal(byId.b.level, 0)
  assert.equal(byId.c.level, 0)   // primary because it is on current_path (higher ordinal than x is irrelevant)
  assert.equal(byId.x.level, 1)   // side branch forks once
  assert.equal(byId.y.level, 1)   // continuation of side branch: same rail
  assert.equal(byId.b.isFork, true)
  assert.deepEqual(rows.map(r => r.node.id), ['a', 'b', 'c', 'x', 'y'])
  assert.equal(byId.c.onPath, true)
  assert.equal(byId.x.onPath, false)
})

test('without current path the lowest ordinal child is primary; nested forks stack levels', () => {
  const nodes = [
    N('a', null, 0), N('b', 'a', 1), N('c', 'a', 2),
    N('d', 'c', 3), N('e', 'c', 4),
  ]
  const rows = buildWorldlineRows(nodes, [], '')
  const byId = Object.fromEntries(rows.map(r => [r.node.id, r]))
  assert.equal(byId.b.level, 0)   // primary child of a
  assert.equal(byId.c.level, 1)   // side branch of a
  assert.equal(byId.d.level, 1)   // primary child of c continues its rail
  assert.equal(byId.e.level, 2)   // second fork stacks one more
})

test('dangling parent ids are treated as roots and never crash', () => {
  const nodes = [N('a', 'ghost', 0), N('b', 'a', 1)]
  const rows = buildWorldlineRows(nodes, null, null)
  assert.deepEqual(rows.map(r => r.node.id), ['a', 'b'])
  assert.deepEqual(rows.map(r => r.level), [0, 0])
  assert.deepEqual(buildWorldlineRows(null, null, null), [])
})

test('worldline edges preserve real parents and only highlight current-path links', () => {
  const nodes = [N('a', null), N('b', 'a', 0), N('c', 'a', 1), N('d', 'c', 0)]
  const rows = buildWorldlineRows(nodes, ['a', 'b'], 'b')
  const edges = buildWorldlineEdges(rows)
  assert.deepEqual(edges.map(edge => edge.id), ['a:b', 'a:c', 'c:d'])
  assert.deepEqual(edges.map(edge => [edge.parentLevel, edge.childLevel]), [[0, 0], [0, 1], [1, 1]])
  assert.deepEqual(edges.map(edge => edge.onPath), [true, false, false])
  assert.equal(edges.some(edge => edge.id === 'b:c'), false)
  assert.deepEqual(buildWorldlineEdges(null), [])
})

test('messageVersionInfo only reports real multi-version groups', () => {
  const wl = { message_versions: {
    'u1': { node_id: 'n1', index: 1, total: 2, next_node_id: 'n2' },
    'u2': { node_id: 'n3', index: 1, total: 1 },
  } }
  assert.equal(messageVersionInfo(wl, 'u1').next_node_id, 'n2')
  assert.equal(messageVersionInfo(wl, 'u2'), null)
  assert.equal(messageVersionInfo(wl, 'u3'), null)
  assert.equal(messageVersionInfo(null, 'u1'), null)
})

test('worldlineNodeTitle extracts text from a truncated structured-content title', () => {
  const title = '[{"type": "text", "text": "\\u65f6\\u95f4\\u7ebf\\u7684\\u6982\\u5ff5\\u662f\\u4ec0\\u4e48\\n\\n---\\n[PROJECT MODE: ga-admin]\\nInjected'
  assert.equal(worldlineNodeTitle({ id: 'v2', title }), '\u65f6\u95f4\u7ebf\u7684\u6982\u5ff5\u662f\u4ec0\u4e48')
})

test('worldlineNodeTitle labels transport state and HTML instead of exposing payloads', () => {
  const state = '[{"text": "{\\"result\\": \\"working key_info updated\\"}\\n\\n### [WORKING MEMORY]'
  const html = '[{"type": "text", "text": "<aside class=\\"oa-worldline-drawer\\">'
  assert.equal(worldlineNodeTitle({ id: 'v1', title: state }), '\u4f1a\u8bdd\u72b6\u6001\u66f4\u65b0')
  assert.equal(worldlineNodeTitle({ id: 'v7', title: html }), '\u754c\u9762\u5185\u5bb9\uff08HTML\uff09')
})

test('worldlineNodeTitle preserves plain titles and provides an empty fallback', () => {
  assert.equal(worldlineNodeTitle({ id: 'v3', title: 'Plain question' }), 'Plain question')
  assert.equal(worldlineNodeTitle({ id: 'abcdef1234', title: '' }), '\u8282\u70b9 abcdef12')
})

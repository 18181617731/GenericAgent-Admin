import test from 'node:test'
import assert from 'node:assert/strict'
import { buildWorldlineRows, worldlineMaxLevel, messageVersionInfo } from './worldlineTree.js'

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

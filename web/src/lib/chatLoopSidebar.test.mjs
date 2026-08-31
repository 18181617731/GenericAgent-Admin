import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { loopSidebarView, updateSessionLoop } from './chatLoopSidebar.js'

test('loopSidebarView exposes active loop progress without a finite cap', () => {
  assert.deepEqual(loopSidebarView({ enabled:true, status:'waiting', round:0, max_rounds:3 }), {
    status:'waiting', round:0,
  })
  assert.deepEqual(loopSidebarView({ enabled:true, status:'evaluating', round:2 }), {
    status:'evaluating', round:2,
  })
  for (const status of ['stopped', 'completed', 'error', 'paused']) {
    assert.equal(loopSidebarView({ enabled:false, status, round:2 }), null)
  }
  assert.equal(loopSidebarView(null), null)
})

test('updateSessionLoop patches only the matching session', () => {
  const sessions = [{ id:'a', loop:null }, { id:'b', title:'Beta' }]
  const loop = { enabled:true, status:'running', round:1 }
  const updated = updateSessionLoop(sessions, 'b', loop)
  assert.equal(updated[0], sessions[0])
  assert.deepEqual(updated[1], { id:'b', title:'Beta', loop })
  assert.equal(sessions[1].loop, undefined)
})

test('ChatApp wires loop updates and progress into the sidebar', () => {
  const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
  assert.match(source, /updateSessionLoop\(xs, sessionId, ev\.loop\)/)
  assert.match(source, /updateSessionLoop\(xs, id, nextLoopState\)/)
  assert.match(source, /loopSidebarView\(session\.loop\)/)
  assert.match(source, /oa-session-loop-badge/)
})

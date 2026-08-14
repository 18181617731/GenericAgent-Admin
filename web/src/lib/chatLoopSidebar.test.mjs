import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { loopSidebarView, updateSessionLoop } from './chatLoopSidebar.js'

test('loopSidebarView exposes only active loop progress', () => {
  assert.deepEqual(loopSidebarView({ enabled:true, status:'waiting', round:0, max_rounds:3 }), {
    status:'waiting', round:0, maxRounds:3,
  })
  assert.deepEqual(loopSidebarView({ enabled:true, status:'evaluating', round:2, max_rounds:4 }), {
    status:'evaluating', round:2, maxRounds:4,
  })
  for (const status of ['stopped', 'completed', 'error', 'paused']) {
    assert.equal(loopSidebarView({ enabled:false, status, round:2, max_rounds:4 }), null)
  }
  assert.equal(loopSidebarView(null), null)
})

test('updateSessionLoop patches only the matching session', () => {
  const sessions = [{ id:'a', loop:null }, { id:'b', title:'Beta' }]
  const loop = { enabled:true, status:'running', round:1, max_rounds:5 }
  const updated = updateSessionLoop(sessions, 'b', loop)
  assert.equal(updated[0], sessions[0])
  assert.deepEqual(updated[1], { id:'b', title:'Beta', loop })
  assert.equal(sessions[1].loop, undefined)
})

test('ChatApp wires loop updates and progress into the sidebar', () => {
  const source = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
  const listSource = readFileSync(new URL('../components/ChatSessionList.jsx', import.meta.url), 'utf8')
  const rowSource = readFileSync(new URL('../components/ChatSessionRow.jsx', import.meta.url), 'utf8')
  assert.match(source, /updateSessionLoop\(xs, sessionId, ev\.loop\)/)
  assert.match(source, /updateSessionLoop\(xs, id, nextLoopState\)/)
  assert.match(listSource, /loopSidebarView\(session\.loop\)/)
  assert.match(rowSource, /oa-session-loop-badge/)
})

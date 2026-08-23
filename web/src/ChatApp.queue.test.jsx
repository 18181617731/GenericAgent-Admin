import { readFileSync } from 'node:fs'
import { describe, expect, test } from 'vitest'

const source = readFileSync('src/ChatApp.jsx', 'utf8')

describe('session-scoped guided-message queue wiring', () => {
  test('clears stale queue immediately, then restores the selected session queue', () => {
    const openStart = source.indexOf('const openSession = async')
    const clearQueue = source.indexOf("syncQueue([], { persist:false })", openStart)
    const fetchSession = source.indexOf('const d = await chatApi(`/api/chat/session/${id}`)', openStart)
    const restoreQueue = source.indexOf("syncQueue(Array.isArray(d.queued_messages) ? d.queued_messages : [], { persist:false })", openStart)
    expect(openStart).toBeGreaterThan(-1)
    expect(clearQueue).toBeGreaterThan(openStart)
    expect(fetchSession).toBeGreaterThan(clearQueue)
    expect(restoreQueue).toBeGreaterThan(fetchSession)
  })

  test('clears both queue state and ref when switching instances', () => {
    const switchStart = source.indexOf('const switchChatInstance =')
    const switchEnd = source.indexOf('\n  }', switchStart)
    const switchSource = source.slice(switchStart, switchEnd)
    expect(switchSource).toContain("syncQueue([], { persist:false })")
    expect(switchSource).not.toContain('setQueuedMessages([])')
  })

  test('serializes full queue snapshots to the session API', () => {
    expect(source).toContain("`/api/chat/queue/${sessionId}`")
    expect(source).toContain("queueWriteRef.current = queueWriteRef.current")
    expect(source).toContain("body:JSON.stringify({ messages:snapshot })")
  })

  test('removes a guided item only after a send has an active session', () => {
    const runStart = source.indexOf('const runSend = async')
    const activeCheck = source.indexOf('} else if (!isActiveSession(id)) {', runStart)
    const removeGuided = source.indexOf('syncQueue(queuedRef.current.filter(x => x.id !== guidedQueueId)', runStart)
    expect(runStart).toBeGreaterThan(-1)
    expect(activeCheck).toBeGreaterThan(runStart)
    expect(removeGuided).toBeGreaterThan(activeCheck)
  })
})

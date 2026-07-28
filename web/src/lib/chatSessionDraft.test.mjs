import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  CHAT_SESSION_DRAFTS_STORAGE_KEY,
  clearChatSessionDrafts,
  listChatSessionDraftIds,
  loadChatSessionDraft,
  saveChatSessionDraft,
} from './chatSessionDrafts.js'

function memoryStorage(initial = {}) {
  const values = new Map(Object.entries(initial))
  return {
    getItem: key => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: key => values.delete(key),
  }
}

function functionBlock(source, start, end) {
  const from = source.indexOf(start)
  assert.notEqual(from, -1, `missing start marker: ${start}`)
  const to = source.indexOf(end, from + start.length)
  assert.notEqual(to, -1, `missing end marker: ${end}`)
  return source.slice(from, to)
}

test('new chat stays out of the session list until its first send', () => {
  const main = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
  const legacy = readFileSync(new URL('../pages/ChatPage.jsx', import.meta.url), 'utf8')

  const mainNewSession = functionBlock(main, '  const newSession = async () => {', '  const deleteSession = async')
  assert.doesNotMatch(mainNewSession, /setSessions\s*\(/)
  assert.doesNotMatch(mainNewSession, /loadSessions\s*\(/)

  const legacyNewSession = functionBlock(legacy, '  const newSession = async () => {', '  useEffect(()')
  assert.doesNotMatch(legacyNewSession, /setSessions\s*\(/)
  assert.doesNotMatch(legacyNewSession, /loadSessions\s*\(/)

  const mainSend = functionBlock(main, '  const runSend = async (item = {}) => {', '  const expandCustomSlashCommand =')
  assert.match(mainSend, /await loadSessions\(id\)/)

  const legacySend = functionBlock(legacy, '  const send = async () => {', '  return <section')
  assert.match(legacySend, /await loadSessions\(\)/)
})

test('chat session drafts persist independently and clear selectively', () => {
  const storage = memoryStorage()

  saveChatSessionDraft('session-a', 'draft A', storage)
  saveChatSessionDraft('session-b', 'draft B', storage)
  assert.deepEqual(listChatSessionDraftIds(storage).sort(), ['session-a', 'session-b'])
  assert.equal(loadChatSessionDraft('session-a', storage), 'draft A')
  assert.equal(loadChatSessionDraft('session-b', storage), 'draft B')

  saveChatSessionDraft('session-a', '', storage)
  assert.deepEqual(listChatSessionDraftIds(storage), ['session-b'])
  assert.equal(loadChatSessionDraft('session-a', storage), '')
  assert.equal(loadChatSessionDraft('session-b', storage), 'draft B')

  clearChatSessionDrafts(['session-b', 'missing'], storage)
  assert.deepEqual(listChatSessionDraftIds(storage), [])
  assert.equal(loadChatSessionDraft('session-b', storage), '')
  assert.equal(storage.getItem(CHAT_SESSION_DRAFTS_STORAGE_KEY), null)
})

test('chat session draft storage failures do not break the composer', () => {
  const corrupt = memoryStorage({ [CHAT_SESSION_DRAFTS_STORAGE_KEY]: '{not-json' })
  assert.equal(loadChatSessionDraft('session-a', corrupt), '')
  assert.doesNotThrow(() => saveChatSessionDraft('session-a', 'recovered', corrupt))
  assert.equal(loadChatSessionDraft('session-a', corrupt), 'recovered')

  const unavailable = {
    getItem() { throw new Error('blocked') },
    setItem() { throw new Error('blocked') },
    removeItem() { throw new Error('blocked') },
  }
  assert.equal(loadChatSessionDraft('session-a', unavailable), '')
  assert.doesNotThrow(() => saveChatSessionDraft('session-a', 'draft', unavailable))
  assert.doesNotThrow(() => clearChatSessionDrafts('session-a', unavailable))
})

test('main chat wires reactive draft badges into persistence, sending, and deletion', () => {
  const main = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
  const style = readFileSync(new URL('../style.css', import.meta.url), 'utf8')
  assert.match(main, /listChatSessionDraftIds/)
  assert.match(main, /loadChatSessionDraft/)
  assert.match(main, /saveChatSessionDraft/)
  assert.match(main, /clearChatSessionDrafts/)
  assert.match(main, /const \[draftSessionIds, setDraftSessionIds\]/)
  assert.equal(main.match(/className="oa-session-draft-badge"/g)?.length, 2)
  assert.match(style, /\.oa-session-title \.oa-session-draft-badge/)
  assert.match(style, /html\[data-theme="dark"\] \.oa-session-title \.oa-session-draft-badge/)

  const openSession = functionBlock(main, '  const openSession = async', '  const loadSessions = async')
  assert.match(openSession, /loadChatSessionDraft\(id\)/)

  const promptSetter = functionBlock(main, '  const setSessionPrompt =', '  useEffect(() => { activeSidRef.current = sid }, [sid])')
  assert.match(promptSetter, /persistSessionDraft\(sessionId, next\)/)

  const promptChange = functionBlock(main, '  const handlePromptChange =', '  const handlePromptKeyDown =')
  assert.match(promptChange, /setSessionPrompt\(v\)/)

  const send = functionBlock(main, '  const send = async', '  const applySlashCommand =')
  assert.match(send, /setSessionPrompt\(''\)/)
  const runSend = functionBlock(main, '  const runSend = async (item = {}) => {', '  const expandCustomSlashCommand =')
  assert.match(runSend, /clearSessionDrafts\(id\)/)

  const deleteSession = functionBlock(main, '  const deleteSession = async', '  const openSessionManager =')
  assert.match(deleteSession, /clearSessionDrafts\(id\)/)
  const batchDelete = functionBlock(main, '  const deleteSelectedSessions = async', '  const startRename =')
  assert.match(batchDelete, /clearSessionDrafts\(result\.deletedIds\)/)
})

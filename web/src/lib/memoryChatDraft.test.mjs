import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { MEMORY_CHAT_DRAFT_STORAGE_KEY, claimMemoryChatDraft, consumeMemoryChatDraft, createMemoryChatDraftSession, queueMemoryChatDraft } from './memoryChatDraft.js'

function installSessionStorage() {
  const values = new Map()
  globalThis.window = {
    sessionStorage: {
      getItem: key => values.get(key) ?? null,
      setItem: (key, value) => values.set(key, String(value)),
      removeItem: key => values.delete(key),
    },
  }
  return values
}

test('memory chat draft is consumed once', () => {
  const values = installSessionStorage()
  const draft = { path: 'memory/example.md', prompt: 'Review this file.' }

  queueMemoryChatDraft(draft)

  assert.deepEqual(consumeMemoryChatDraft(), draft)
  assert.equal(values.has(MEMORY_CHAT_DRAFT_STORAGE_KEY), false)
  assert.equal(consumeMemoryChatDraft(), null)
})

test('invalid memory chat draft is cleared', () => {
  const values = installSessionStorage()
  values.set(MEMORY_CHAT_DRAFT_STORAGE_KEY, '{not-json')

  assert.equal(consumeMemoryChatDraft(), null)
  assert.equal(values.has(MEMORY_CHAT_DRAFT_STORAGE_KEY), false)
})

test('a memory draft creates exactly one session across A to B to A instance switches', async () => {
  const draft = { path: 'memory/example.md', prompt: 'Review this file.' }
  const draftRef = { current: draft }
  let createCount = 0
  const createSession = async () => `session-${++createCount}`

  assert.equal(claimMemoryChatDraft(draftRef, ''), null)
  assert.equal(draftRef.current, draft)
  assert.deepEqual(await createMemoryChatDraftSession(draftRef, createSession), {
    draft,
    sessionID: 'session-1',
  })
  assert.equal(draftRef.current, null)
  assert.equal(await createMemoryChatDraftSession(draftRef, createSession), null)
  assert.equal(await createMemoryChatDraftSession(draftRef, createSession), null)
  assert.equal(createCount, 1)
})

test('full chat workspace starts a fresh session with the memory draft', () => {
  const chatApp = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8').replaceAll('\r\n', '\n')

  assert.match(chatApp, /import \{ consumeMemoryChatDraft, createMemoryChatDraftSession \} from '\.\/lib\/memoryChatDraft\.js'/)
  assert.match(chatApp, /const memoryDraftRef = useRef\(consumeMemoryChatDraft\(\)\)/)
  assert.match(chatApp, /if \(draft\) \{\n\s*const claimed = await createMemoryChatDraftSession\(memoryDraftRef, createSession\)/)
  assert.match(chatApp, /setSessionPrompt\(claimed\.draft\.prompt, claimed\.sessionID\)/)
  assert.match(chatApp, /Review the draft before sending/)
})

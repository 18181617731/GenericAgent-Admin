import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { MEMORY_CHAT_DRAFT_STORAGE_KEY, consumeMemoryChatDraft, queueMemoryChatDraft } from './memoryChatDraft.js'

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

test('full chat workspace starts a fresh session with the memory draft', () => {
  const chatApp = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8').replaceAll('\r\n', '\n')

  assert.match(chatApp, /import \{ consumeMemoryChatDraft \} from '\.\/lib\/memoryChatDraft\.js'/)
  assert.match(chatApp, /const memoryDraftRef = useRef\(consumeMemoryChatDraft\(\)\)/)
  assert.match(chatApp, /if \(draft\) \{\n\s*const newSessionID = await createSession\(\)/)
  assert.match(chatApp, /setSessionPrompt\(draft\.prompt, newSessionID\)/)
  assert.match(chatApp, /Review the draft before sending/)
})

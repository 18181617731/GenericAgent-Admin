import test from 'node:test'
import assert from 'node:assert/strict'
import {
  CHAT_PRIVACY_STORAGE_KEY,
  chatNotificationForDisplay,
  loadChatPrivacyMode,
  normalizeChatPrivacyMode,
  privateProjectTitle,
  privateSessionTitle,
  saveChatPrivacyMode,
  subscribeChatPrivacyMode,
} from './chatPrivacy.js'

const memoryStorage = initial => {
  const values = new Map(Object.entries(initial || {}))
  return {
    getItem:key => values.has(key) ? values.get(key) : null,
    setItem:(key, value) => values.set(key, String(value)),
    removeItem:key => values.delete(key),
  }
}

const eventWindow = () => {
  const listeners = new Map()
  return {
    addEventListener:(name, listener) => listeners.set(name, listener),
    removeEventListener:(name, listener) => { if (listeners.get(name) === listener) listeners.delete(name) },
    dispatchEvent:event => { listeners.get(event.type)?.(event); return true },
    emit:(name, event) => listeners.get(name)?.(event),
  }
}

test('privacy mode defaults off and only accepts explicit true', () => {
  assert.equal(loadChatPrivacyMode(memoryStorage()), false)
  assert.equal(loadChatPrivacyMode(memoryStorage({ [CHAT_PRIVACY_STORAGE_KEY]:'invalid' })), false)
  assert.equal(normalizeChatPrivacyMode(true), true)
  assert.equal(normalizeChatPrivacyMode('true'), true)
  assert.equal(normalizeChatPrivacyMode('1'), false)
})

test('privacy mode persists and synchronizes custom and storage events', () => {
  const storage = memoryStorage()
  const windowRef = eventWindow()
  const updates = []
  const unsubscribe = subscribeChatPrivacyMode(value => updates.push(value), { windowRef })

  assert.equal(saveChatPrivacyMode(true, { storage, windowRef }), true)
  assert.equal(loadChatPrivacyMode(storage), true)
  windowRef.emit('storage', { key:CHAT_PRIVACY_STORAGE_KEY, newValue:'false' })
  assert.deepEqual(updates, [true, false])

  unsubscribe()
  windowRef.emit('storage', { key:CHAT_PRIVACY_STORAGE_KEY, newValue:'true' })
  assert.deepEqual(updates, [true, false])
})

test('private labels are stable and contain no source content', () => {
  assert.equal(privateSessionTitle(2, 'zh'), '会话 03')
  assert.equal(privateSessionTitle(2, 'en'), 'Session 03')
  assert.equal(privateProjectTitle(0, 'zh'), '项目 01')
})

test('chat notification redaction preserves state but removes sensitive preview', () => {
  const raw = { id:'n1', category:'chat', level:'error', title:'SECRET_TITLE', message:'SECRET_PROMPT D:/secret.txt' }
  const hidden = chatNotificationForDisplay(raw, true, 'zh')
  assert.equal(hidden.title, '对话任务失败')
  assert.equal(hidden.message, '当前视图已收起详情。')
  assert.equal(hidden.id, raw.id)
  assert.equal(chatNotificationForDisplay(raw, false, 'zh'), raw)

  const stopped = chatNotificationForDisplay({ ...raw, level:'warning' }, true, 'zh')
  assert.equal(stopped.title, '对话任务已停止')

  const system = { category:'system', level:'error', title:'系统错误', message:'保留详情' }
  assert.equal(chatNotificationForDisplay(system, true, 'zh'), system)
})

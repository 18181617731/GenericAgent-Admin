import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

const chatSource = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
const notificationsSource = readFileSync(new URL('./notifications.js', import.meta.url), 'utf8')
const styleSource = readFileSync(new URL('../style.css', import.meta.url), 'utf8')

test('privacy mode unmounts sensitive chat surfaces instead of visually blurring them', () => {
  assert.match(chatSource, /privacyMode \? <ChatPrivacyCurtain[\s\S]*: <MessageList/)
  assert.match(chatSource, /!privacyMode && contextOpen/)
  assert.match(chatSource, /!privacyMode && worldlineOpen/)
  assert.match(chatSource, /open=\{sessionSearchOpen && !privacyMode\}/)
  assert.match(chatSource, /!privacyMode && <SubagentStatusPanel/)
  assert.doesNotMatch(chatSource, /filter:\s*blur\(|backdrop-filter:\s*blur\([^)]*privacy/i)
})

test('browser notifications are redacted before every delivery fallback', () => {
  const redactIndex = notificationsSource.indexOf('const displayItem = chatNotificationForDisplay')
  const deliveryIndex = notificationsSource.indexOf('registration.showNotification(displayItem.title')
  assert.ok(redactIndex >= 0)
  assert.ok(deliveryIndex > redactIndex)
  assert.match(notificationsSource, /showWindowNotification\(displayItem\)/)
})

test('mobile sidebar keeps the complete drawer reachable with one touch scroll', () => {
  assert.match(styleSource, /\.oa-chat \.oa-sidebar\s*\{[\s\S]*?display:\s*flex !important;[\s\S]*?overflow-y:\s*auto !important;[\s\S]*?touch-action:\s*pan-y;/)
  assert.match(styleSource, /\.oa-chat \.oa-session-list\s*\{[\s\S]*?max-height:\s*none;[\s\S]*?overflow:\s*visible;/)
})

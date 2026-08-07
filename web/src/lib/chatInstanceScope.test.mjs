import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  addChatInstanceToURL,
  chatInstanceOptions,
  initialChatInstanceID,
  persistChatInstanceID,
} from './chatInstanceScope.js'

const chatSource = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
const chatStyles = readFileSync(new URL('../style.css', import.meta.url), 'utf8')

test('renders the instance selector once at the top of the session sidebar', () => {
  assert.equal((chatSource.match(/className="oa-sidebar-instance"/g) || []).length, 1)
  assert.equal((chatSource.match(/aria-label=\{ct\('选择 GA 实例'/g) || []).length, 1)
  const sidebar = chatSource.indexOf('<aside className={`oa-sidebar')
  const selector = chatSource.indexOf('className="oa-sidebar-instance"', sidebar)
  const search = chatSource.indexOf('className="oa-sidebar-search"', sidebar)
  const sidebarEnd = chatSource.indexOf('</aside>', sidebar)
  assert.ok(sidebar >= 0)
  assert.ok(selector > sidebar && selector < search)
  assert.ok(search < sidebarEnd)
  assert.doesNotMatch(chatSource, /oa-instance-select|oa-mobile-instance-select/)
  assert.doesNotMatch(chatSource, /刷新会话|Refresh sessions|RefreshCw/)
  assert.equal((chatStyles.match(/grid-template-columns:\s*320px minmax\(0,1fr\)/g) || []).length, 2)
  assert.equal((chatStyles.match(/\.oa-sidebar\s*\{[^}]*width:\s*320px/g) || []).length, 2)
  assert.match(chatStyles, /width:min\(90vw,340px\)\s*!important/)
})

test('addChatInstanceToURL scopes only chat API routes and preserves query/hash', () => {
  assert.equal(addChatInstanceToURL('/api/chat/sessions', ' ga-2 '), '/api/chat/sessions?instance_id=ga-2')
  assert.equal(addChatInstanceToURL('/api/chat/stream/s1?from=7#tail', 'ga 2'), '/api/chat/stream/s1?from=7&instance_id=ga+2#tail')
  assert.equal(addChatInstanceToURL('/api/instances', 'ga-2'), '/api/instances')
  assert.equal(addChatInstanceToURL('/api/chat/sessions', ''), '/api/chat/sessions')
})

test('initialChatInstanceID prefers the tab URL over session storage', () => {
  const storage = { getItem: () => 'stored-instance' }
  assert.equal(initialChatInstanceID({ location: { search: '?instance_id=url-instance' }, storage }), 'url-instance')
  assert.equal(initialChatInstanceID({ location: { search: '' }, storage }), 'stored-instance')
})

test('persistChatInstanceID updates tab storage and current URL', () => {
  const calls = []
  const storage = {
    setItem: (key, value) => calls.push(['set', key, value]),
    removeItem: key => calls.push(['remove', key]),
  }
  const history = { state: { keep: true }, replaceState: (...args) => calls.push(['replace', ...args]) }
  const location = { href: 'http://localhost/chat?keep=1#thread' }

  persistChatInstanceID('ga-3', { history, location, storage })
  assert.deepEqual(calls[0], ['set', 'ga-admin-chat-instance-id', 'ga-3'])
  assert.deepEqual(calls[1], ['replace', history.state, '', '/chat?keep=1&instance_id=ga-3#thread'])
})

test('chatInstanceOptions normalizes names and initialization state', () => {
  assert.deepEqual(chatInstanceOptions({ items: [
    { id: 'alpha', name: 'Alpha', init_status: 'ready' },
    { id: ' beta ', name: '', init_status: 'INITIALIZING' },
    { id: '', name: 'ignored' },
  ] }), [
    { id: 'alpha', name: 'Alpha', initializing: false },
    { id: 'beta', name: 'beta', initializing: true },
  ])
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import {
  clearChatLaunchIntent,
  navigateToNewChat,
  newChatLaunchURL,
  readChatLaunchIntent,
} from './chatLaunchIntent.js'

const chatSource = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')
const overviewSource = readFileSync(new URL('../pages/OverviewPage.jsx', import.meta.url), 'utf8')

test('builds a new-chat URL with a reviewable composer prompt', () => {
  assert.equal(newChatLaunchURL('/update'), '/?new=1&prompt=%2Fupdate')
  const location = { href: '' }
  navigateToNewChat('/update', { location })
  assert.equal(location.href, '/?new=1&prompt=%2Fupdate')
})

test('reads and clears a launch intent without losing unrelated URL state', () => {
  const location = {
    search: '?instance_id=ga-2&new=1&prompt=%2Fupdate&keep=yes',
    href: 'http://localhost/?instance_id=ga-2&new=1&prompt=%2Fupdate&keep=yes#composer',
  }
  assert.deepEqual(readChatLaunchIntent({ location }), { newChat: true, prompt: '/update' })

  const calls = []
  const history = { state: { keep: true }, replaceState: (...args) => calls.push(args) }
  clearChatLaunchIntent({ history, location })
  assert.deepEqual(calls, [[history.state, '', '/?instance_id=ga-2&keep=yes#composer']])
})

test('the overview CTA and chat startup share the one-shot launch contract', () => {
  assert.match(overviewSource, /navigateToNewChat\('\/update'\)/)
  assert.match(chatSource, /useRef\(readChatLaunchIntent\(\)\)/)
  assert.match(chatSource, /if \(chatInstancesLoading\) return/)
  assert.match(chatSource, /openedChatInstanceRef\.current === instanceKey/)
  assert.match(chatSource, /await createSession\(\)/)
  assert.match(chatSource, /setSessionPrompt\(intent\.prompt, newSessionID\)/)
  assert.match(chatSource, /clearChatLaunchIntent\(\)/)
})

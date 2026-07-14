import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'

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

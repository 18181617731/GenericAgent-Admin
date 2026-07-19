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

const main = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8').replaceAll('\r\n', '\n')

test('prompt preset picker opens immediately and refreshes one stable session in background', () => {
  const open = functionBlock(main, '  const openExtraPromptEditor = () =>', '  const openPromptPresetManager =')
  assert.match(open, /const targetSid = activeSidRef\.current/)
  assert.match(open, /const targetOpenToken = openSeqRef\.current/)
  assert.match(open, /const initialSelection = extraSysPromptPresetID/)
  assert.match(open, /setExtraPromptTargetSid\(targetSid\)/)
  assert.match(open, /setExtraPromptSelection\(initialSelection\)/)
  assert.match(open, /setExtraPromptOpen\(true\)/)
  assert.match(open, /loadChatState\(targetSid, targetOpenToken\)/)
  assert.match(open, /targetOpenToken !== openSeqRef\.current \|\| activeSidRef\.current !== targetSid/)
  assert.match(open, /current === initialSelection \? freshState\.extraSysPromptPresetID : current/)
  assert.ok(open.indexOf('setExtraPromptOpen(true)') < open.indexOf('Promise.all('))
})

test('prompt preset save cannot target or update a different active session', () => {
  const save = functionBlock(main, '  const saveExtraPromptSelection = async', '  const savePromptPresets = async')
  assert.match(save, /const targetSid = extraPromptTargetSid/)
  assert.match(save, /if \(activeSidRef\.current !== targetSid\)/)
  assert.ok(save.includes('api(`/api/chat/settings/${targetSid}`'))
  assert.match(save, /targetOpenToken !== openSeqRef\.current \|\| activeSidRef\.current !== targetSid/)
})

test('prompt presets load on initial render so a saved selection has its real name', () => {
  const mount = functionBlock(main, "  useEffect(() => {\n    loadSessions('', { open:true })", '  useEffect(() => {\n    let stopped')
  assert.match(mount, /loadPromptPresets\(\)\.catch/)
})

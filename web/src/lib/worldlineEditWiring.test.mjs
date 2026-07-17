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

const main = readFileSync(new URL('../ChatApp.jsx', import.meta.url), 'utf8')

test('ChatApp routes edit-resend through the normal same-session send path', () => {
  assert.match(main, /import \{ buildChatRunPayload, buildEditResendItem \} from '\.\/lib\/worldlineEdit'/)

  const editResend = functionBlock(main, '  const handleEditResend = async', '  const loadChatState = async')
  assert.match(editResend, /buildEditResendItem\(/)
  assert.match(editResend, /await runSend\(item\)/)
  assert.doesNotMatch(editResend, /\/api\/chat\/fork/)

  const runSend = functionBlock(main, '  const runSend = async (item = {}) => {', '  const expandCustomSlashCommand =')
  assert.match(runSend, /buildChatRunPayload\(/)
  assert.match(runSend, /sourceUserMessageId:\s*item\.sourceUserMessageId/)
  assert.match(runSend, /if \(item\.propagateError\) throw e/)
})

test('message editor only closes after edit-resend succeeds', () => {
  const submitEdit = functionBlock(main, '  const submitEdit = async () => {', '  const copyContent = () => {')
  assert.match(submitEdit, /await onEditResend\?\.\(/)
  assert.match(submitEdit, /setEditing\(false\)/)
  assert.match(submitEdit, /catch/)
})

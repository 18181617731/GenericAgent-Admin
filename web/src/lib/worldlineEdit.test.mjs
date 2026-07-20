import test from 'node:test'
import assert from 'node:assert/strict'
import { buildChatRunPayload, buildEditResendItem } from './worldlineEdit.js'

test('edit-resend keeps the current SID, identifies the source message, and clears attachments', () => {
  const item = buildEditResendItem({ sessionId:'same-sid', messageId:'user-7', text:'edited prompt' })
  assert.deepEqual(item, {
    text:'edited prompt',
    files:[],
    sessionId:'same-sid',
    sourceUserMessageId:'user-7',
    propagateError:true,
  })
  const payload = buildChatRunPayload({
    prompt:item.text,
    files:item.files,
    settings:{ llm_no:2 },
    clientUserID:'client-1',
    sourceUserMessageId:item.sourceUserMessageId,
  })
  assert.equal(payload.source_user_message_id, 'user-7')
  assert.deepEqual(payload.files, [])
})

test('edit-resend rejects the current busy session but not an unrelated busy session', () => {
  assert.throws(
    () => buildEditResendItem({ sessionId:'same-sid', messageId:'user-7', text:'x', busy:true, streamingSid:'same-sid' }),
    /\u8fd0\u884c\u4e2d/,
  )
  assert.equal(buildEditResendItem({ sessionId:'same-sid', messageId:'user-7', text:'x', busy:true, streamingSid:'other-sid' }).sessionId, 'same-sid')
})

test('edit-resend rejects missing session or source-message identity', () => {
  assert.throws(() => buildEditResendItem({ messageId:'user-7', text:'x' }), /\u65e0\u6cd5\u5b9a\u4f4d/)
  assert.throws(() => buildEditResendItem({ sessionId:'same-sid', text:'x' }), /\u65e0\u6cd5\u5b9a\u4f4d/)
})

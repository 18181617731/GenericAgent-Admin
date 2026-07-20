export const buildChatRunPayload = ({ prompt, files = [], settings = {}, clientUserID = '', sourceUserMessageId = '' } = {}) => ({
  prompt,
  files,
  settings,
  client_user_id: clientUserID,
  source_user_message_id: String(sourceUserMessageId || '').trim() || undefined,
})

export const buildEditResendItem = ({ sessionId, messageId, text, busy = false, streamingSid = '' } = {}) => {
  const sourceSessionID = String(sessionId || '').trim()
  const sourceMessageID = String(messageId || '').trim()
  if (!sourceSessionID || !sourceMessageID) throw new Error('\u65e0\u6cd5\u5b9a\u4f4d\u8981\u7f16\u8f91\u7684\u5386\u53f2\u6d88\u606f')
  if (busy && String(streamingSid || '').trim() === sourceSessionID) throw new Error('\u5bf9\u8bdd\u8fd0\u884c\u4e2d\uff0c\u8bf7\u7b49\u5f85\u5b8c\u6210\u540e\u518d\u7f16\u8f91')
  return {
    text,
    files: [],
    sessionId: sourceSessionID,
    sourceUserMessageId: sourceMessageID,
    propagateError: true,
  }
}

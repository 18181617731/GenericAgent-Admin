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
  if (!sourceSessionID || !sourceMessageID) throw new Error('无法定位要编辑的历史消息')
  if (busy && String(streamingSid || '').trim() === sourceSessionID) throw new Error('对话运行中，请等待完成后再编辑')
  return {
    text,
    files: [],
    sessionId: sourceSessionID,
    sourceUserMessageId: sourceMessageID,
    propagateError: true,
  }
}

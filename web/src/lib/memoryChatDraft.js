export const MEMORY_CHAT_DRAFT_STORAGE_KEY = 'generic-agent-admin:memory-chat-draft'

export const queueMemoryChatDraft = (draft) => {
  window.sessionStorage.setItem(MEMORY_CHAT_DRAFT_STORAGE_KEY, JSON.stringify(draft))
}

export const clearMemoryChatDraft = () => window.sessionStorage.removeItem(MEMORY_CHAT_DRAFT_STORAGE_KEY)

export const consumeMemoryChatDraft = () => {
  const raw = window.sessionStorage.getItem(MEMORY_CHAT_DRAFT_STORAGE_KEY)
  clearMemoryChatDraft()
  if (!raw) return null
  try {
    const draft = JSON.parse(raw)
    return typeof draft?.prompt === 'string' && draft.prompt.trim() ? draft : null
  } catch {
    return null
  }
}

export const claimMemoryChatDraft = (draftRef, sessionID) => {
  if (!sessionID || !draftRef?.current) return null
  const draft = draftRef.current
  draftRef.current = null
  return draft
}

export const createMemoryChatDraftSession = async (draftRef, createSession) => {
  if (!draftRef?.current || typeof createSession !== 'function') return null
  const sessionID = await createSession()
  const draft = claimMemoryChatDraft(draftRef, sessionID)
  return draft ? { draft, sessionID } : null
}

export const shouldPollGeneratedTitle = (session) => session?.title_source === 'temporary'

export async function pollGeneratedChatTitle({
  sessionId,
  loadSessions,
  isActive = () => true,
  wait = (delay) => new Promise(resolve => setTimeout(resolve, delay)),
  delay = 750,
  maxAttempts = 20,
}) {
  let current = null
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    if (!isActive(sessionId)) return current
    await wait(delay)
    if (!isActive(sessionId)) return current
    const sessions = await loadSessions(sessionId)
    current = sessions.find(session => session.id === sessionId) || null
    if (!shouldPollGeneratedTitle(current)) return current
  }
  return current
}

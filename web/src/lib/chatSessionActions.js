export const lastUserMessageID = messages => {
  const list = Array.isArray(messages) ? messages : []
  return [...list].reverse().find(message => message?.role === 'user' && message?.id)?.id || ''
}

export const nextActiveSession = (sessions, excludedID) => {
  const id = String(excludedID || '')
  return (Array.isArray(sessions) ? sessions : []).find(session => session?.id && session.id !== id && !session.archived)?.id || ''
}

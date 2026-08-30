export const newChatLaunchURL = prompt => {
  const params = new URLSearchParams()
  params.set('new', '1')
  const value = String(prompt || '')
  if (value) params.set('prompt', value)
  return `/?${params.toString()}`
}

export const navigateToNewChat = (prompt, { location = window.location } = {}) => {
  location.href = newChatLaunchURL(prompt)
}

export const readChatLaunchIntent = ({ location = window.location } = {}) => {
  const params = new URLSearchParams(location.search)
  return {
    newChat: params.get('new') === '1',
    prompt: params.get('prompt') || '',
  }
}

export const clearChatLaunchIntent = ({ history = window.history, location = window.location } = {}) => {
  const next = new URL(location.href)
  next.searchParams.delete('new')
  next.searchParams.delete('prompt')
  history.replaceState(history.state, '', `${next.pathname}${next.search}${next.hash}`)
}

import { getSelectedInstanceID, normalizeInstanceID, setSelectedInstanceID } from './instanceScope.js'

const CHAT_INSTANCE_STORAGE_KEY = 'ga-admin-chat-instance-id'

export const normalizeChatInstanceID = normalizeInstanceID

export const addChatInstanceToURL = (url, instanceID) => {
  const id = normalizeChatInstanceID(instanceID)
  if (!id || !String(url || '').startsWith('/api/chat')) return url
  const parsed = new URL(url, 'http://ga-admin.local')
  parsed.searchParams.set('instance_id', id)
  return `${parsed.pathname}${parsed.search}${parsed.hash}`
}

export const requestChatInstance = (request, instanceID, url, options) =>
  request(addChatInstanceToURL(url, instanceID), options)

export const initialChatInstanceID = ({ location = window.location, storage = window.sessionStorage } = {}) => {
  const fromURL = normalizeChatInstanceID(new URLSearchParams(location.search).get('instance_id'))
  if (fromURL) return fromURL
  const fromGlobalStorage = getSelectedInstanceID()
  if (fromGlobalStorage) return fromGlobalStorage
  try { return normalizeChatInstanceID(storage.getItem(CHAT_INSTANCE_STORAGE_KEY)) } catch { return '' }
}

export const persistChatInstanceID = (instanceID, { history = window.history, location = window.location, storage = window.sessionStorage } = {}) => {
  const id = normalizeChatInstanceID(instanceID)
  try {
    if (id) storage.setItem(CHAT_INSTANCE_STORAGE_KEY, id)
    else storage.removeItem(CHAT_INSTANCE_STORAGE_KEY)
  } catch { /* session storage can be unavailable in hardened browsers */ }
  setSelectedInstanceID(id)
  const next = new URL(location.href)
  if (id) next.searchParams.set('instance_id', id)
  else next.searchParams.delete('instance_id')
  history.replaceState(history.state, '', `${next.pathname}${next.search}${next.hash}`)
}

export const chatInstanceOptions = payload => {
  const items = Array.isArray(payload?.items) ? payload.items : []
  return items.map(item => ({
    id: normalizeChatInstanceID(item?.id),
    name: String(item?.name || item?.id || '').trim(),
    initializing: String(item?.init_status || '').trim().toLowerCase() === 'initializing',
  })).filter(item => item.id)
}

export { CHAT_INSTANCE_STORAGE_KEY }

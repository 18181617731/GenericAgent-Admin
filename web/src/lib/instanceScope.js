export const INSTANCE_STORAGE_KEY = 'ga-admin-instance-id'
export const INSTANCE_HEADER = 'X-GA-Instance-ID'

export const normalizeInstanceID = value => String(value || '').trim()

const browserStorage = () => {
  if (typeof window === 'undefined') return null
  try { return window.localStorage } catch { return null }
}

export const getSelectedInstanceID = (storage = browserStorage()) => {
  try { return normalizeInstanceID(storage?.getItem(INSTANCE_STORAGE_KEY)) } catch { return '' }
}

export const setSelectedInstanceID = (instanceID, { storage = browserStorage(), dispatch = true } = {}) => {
  const id = normalizeInstanceID(instanceID)
  try {
    if (id) storage?.setItem(INSTANCE_STORAGE_KEY, id)
    else storage?.removeItem(INSTANCE_STORAGE_KEY)
  } catch { /* local storage can be unavailable in hardened browsers */ }
  if (dispatch && typeof window !== 'undefined') {
    window.dispatchEvent(new CustomEvent('ga-admin-instance-change', { detail: id }))
  }
  return id
}

// Add the selected instance only to backend API URLs. Static assets and
// browser routes remain untouched, while legacy callers can keep using their
// existing relative URL construction.
export const addInstanceToURL = (url, instanceID) => {
  const raw = String(url || '')
  const parsed = raw.startsWith('/api/') ? new URL(raw, 'http://ga-admin.local') : null
  const id = normalizeInstanceID(instanceID) || normalizeInstanceID(parsed?.searchParams.get('instance_id')) || getSelectedInstanceID()
  if (!id || !raw.startsWith('/api/')) return url
  parsed.searchParams.set('instance_id', id)
  return `${parsed.pathname}${parsed.search}${parsed.hash}`
}

export const instanceURL = addInstanceToURL

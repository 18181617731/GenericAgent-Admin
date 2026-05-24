export const api = async (url, options = {}) => {
  const { dangerous = false, headers = {}, ...rest } = options
  const res = await fetch(url, { headers: { 'Content-Type': 'application/json', ...(dangerous ? { 'X-GA-Confirm': 'dangerous' } : {}), ...headers }, ...rest })
  const text = await res.text()
  let body = null
  try { body = text ? JSON.parse(text) : null } catch { throw new Error(`Expected JSON from ${url}, got ${text.slice(0, 40)}`) }
  if (!res.ok) throw new Error(body?.detail || `${res.status} ${res.statusText}`)
  return body
}

const JSON_CONTENT_TYPE = 'application/json'
const DANGEROUS_CONFIRM_HEADER = 'X-GA-Confirm'
const DANGEROUS_CONFIRM_VALUE = 'dangerous'

const isFormBody = (body) => typeof FormData !== 'undefined' && body instanceof FormData

export const apiHeaders = ({ dangerous = false, headers = {}, body } = {}) => {
  const normalized = { ...(dangerous ? { [DANGEROUS_CONFIRM_HEADER]: DANGEROUS_CONFIRM_VALUE } : {}), ...headers }
  if (!isFormBody(body) && !Object.keys(normalized).some(k => k.toLowerCase() === 'content-type')) {
    normalized['Content-Type'] = JSON_CONTENT_TYPE
  }
  return normalized
}

export const parseApiResponse = async (res, url = '') => {
  const text = await res.text()
  let body = null
  if (text) {
    try { body = JSON.parse(text) }
    catch {
      if (!res.ok) throw new Error(text.slice(0, 200) || `${res.status} ${res.statusText}`)
      throw new Error(`Expected JSON from ${url}, got ${text.slice(0, 40)}`)
    }
  }
  if (!res.ok) throw new Error(body?.detail || body?.error || text || `${res.status} ${res.statusText}`)
  return body
}

export const readableApiError = (error, url = '') => {
  if (error?.name === 'AbortError') return error
  const message = String(error?.message || error || '')
  if (error instanceof TypeError || /failed to fetch|networkerror|load failed/i.test(message)) {
    return new Error(`无法连接 GA Admin 服务，请确认服务仍在运行后重试${url ? `（${url}）` : ''}`)
  }
  return error instanceof Error ? error : new Error(message || '请求失败')
}

export const api = async (url, options = {}) => {
  const { dangerous = false, headers = {}, ...rest } = options
  const req = { ...rest, headers: apiHeaders({ dangerous, headers, body: rest.body }) }
  try { return await parseApiResponse(await fetch(url, req), url) }
  catch (error) { throw readableApiError(error, url) }
}

export const apiStream = async (url, options = {}) => {
  const { dangerous = false, headers = {}, ...rest } = options
  const req = { ...rest, headers: apiHeaders({ dangerous, headers, body: rest.body }) }
  try {
    const res = await fetch(url, req)
    if (!res.ok) await parseApiResponse(res, url)
    return res
  } catch (error) { throw readableApiError(error, url) }
}

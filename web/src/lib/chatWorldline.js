export const WORLDLINE_SCHEMA_VERSION = 1
export const WORLDLINE_RENDER_NODE_LIMIT = 500

export const emptyWorldlineState = (sid = '') => ({
  sid,
  status: sid ? 'loading' : 'idle',
  data: null,
  error: '',
  switchingNodeId: '',
})

export const normalizeWorldline = (value) => {
  const source = value && typeof value === 'object' ? value : {}
  const schemaVersion = source.schema_version == null ? WORLDLINE_SCHEMA_VERSION : Number(source.schema_version)
  const sourceNodes = Array.isArray(source.nodes) ? source.nodes.filter(node => node?.id) : []
  const nodes = sourceNodes.slice(0, WORLDLINE_RENDER_NODE_LIMIT)
  const truncated = sourceNodes.length > nodes.length || source.truncated === true
  const nodeIds = new Set(nodes.map(node => String(node.id)))
  const currentPath = Array.isArray(source.current_path)
    ? source.current_path.map(String).filter(id => nodeIds.has(id))
    : []
  const head = source.head == null ? '' : String(source.head)
  return {
    schema_version: schemaVersion,
    available: source.available === true,
    degraded_reason: String(source.degraded_reason || (truncated ? 'topology-truncated' : '')),
    truncated,
    root_id: source.root_id == null ? '' : String(source.root_id),
    head,
    current_path: currentPath,
    nodes,
    message_versions: source.message_versions && typeof source.message_versions === 'object' ? source.message_versions : {},
    assistant_message_ids: source.assistant_message_ids && typeof source.assistant_message_ids === 'object' ? source.assistant_message_ids : {},
  }
}

export const classifyWorldline = (value) => {
  const data = normalizeWorldline(value)
  if (data.schema_version !== WORLDLINE_SCHEMA_VERSION) return 'schema-error'
  if (!data.available) return data.degraded_reason ? 'degraded' : 'unavailable'
  if (!data.nodes.length) return 'empty'
  return 'ready'
}

export const worldlineVersionForMessage = (worldline, messageId) => {
  const data = normalizeWorldline(worldline)
  const key = String(messageId || '')
  const direct = data.message_versions[key]
  if (direct) return direct
  const userID = Object.keys(data.assistant_message_ids).find(id => String(data.assistant_message_ids[id]) === key)
  return userID ? data.message_versions[userID] || null : null
}

export const worldlineNodeMap = (worldline) => new Map(normalizeWorldline(worldline).nodes.map(node => [String(node.id), node]))

export const applyWorldlineResponse = (previous, response, sid) => {
  const data = normalizeWorldline(response?.worldline ?? response)
  const status = classifyWorldline(data)
  if (status === 'schema-error') {
    return {
      sid,
      status: previous?.data ? 'stale-error' : 'schema-error',
      data: previous?.data || null,
      error: `Unsupported worldline schema version: ${data.schema_version}`,
      switchingNodeId: '',
    }
  }
  return { sid, status, data, error: '', switchingNodeId: '' }
}

export const worldlineErrorState = (previous, error, sid) => ({
  sid,
  status: previous?.data ? 'stale-error' : 'error',
  data: previous?.data || null,
  error: error?.message || String(error || 'Worldline request failed'),
  switchingNodeId: '',
})

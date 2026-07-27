// worldlineTree.js — pure helpers for the persistent worldline branch panel.
// Level rule (project memory L15): nodes on one continuous chain share the same
// rail level; only a fork side-branch gains +1 level. Never indent per parent_id.

function decodeJSONStringFragment(value) {
  return String(value || '').replace(/\\(u[0-9a-fA-F]{4}|["\\/bfnrt])/g, (_, escape) => {
    if (escape[0] === 'u') return String.fromCharCode(Number.parseInt(escape.slice(1), 16))
    return { '"': '"', '\\': '\\', '/': '/', b: '\b', f: '\f', n: '\n', r: '\r', t: '\t' }[escape]
  })
}

function cleanWorldlineTitle(value) {
  let text = String(value || '').trim()
  const encodedText = text.match(/(?:^|[,\[{]\s*)["']text["']\s*:\s*["']([\s\S]*)$/)
  if (encodedText) text = decodeJSONStringFragment(encodedText[1]).replace(/["'}\]]+\s*$/, '')
  text = text.split(/\n\s*---\s*\n\[PROJECT MODE:/i, 1)[0].trim()
  return text.replace(/\s+/g, ' ').trim()
}

export function worldlineNodeTitle(node) {
  const raw = String(node?.title || '').trim()
  const text = cleanWorldlineTitle(raw)
  if (!text) return `节点 ${String(node?.id || '').slice(0, 8) || '未知'}`
  if (/^\{?\s*["']result["']\s*:/.test(text) || /###\s*\[WORKING MEMORY\]/i.test(text)) return '会话状态更新'
  if (/^\s*</.test(text)) return '界面内容（HTML）'
  if (/^[\[{]\s*["'](?:type|text|content|result)["']\s*:/.test(text)) return '历史消息'
  return text
}

// Node kind is exposed verbatim by the API; only origin/edit are produced today.
const WORLDLINE_KIND_LABELS = { origin: '起点', edit: '编辑', current: '当前' }

export function worldlineNodeKindLabel(node) {
  const kind = String(node?.kind || '').trim()
  if (!kind) return ''
  return WORLDLINE_KIND_LABELS[kind] || kind
}

function nodeOrd(a, b) {
  const ao = Number.isFinite(a?.ordinal) ? a.ordinal : 0
  const bo = Number.isFinite(b?.ordinal) ? b.ordinal : 0
  if (ao !== bo) return ao - bo
  const at = Number.isFinite(a?.created_at) ? a.created_at : 0
  const bt = Number.isFinite(b?.created_at) ? b.created_at : 0
  if (at !== bt) return at - bt
  return String(a?.id || '').localeCompare(String(b?.id || ''))
}

// Returns ordered rows: [{ node, level, onPath, isCurrent, isFork }]
// - DFS pre-order, primary child first (primary = child on currentPath, else lowest ordinal).
// - Primary child inherits parent level; every other sibling gets level+1.
export function buildWorldlineRows(nodes, currentPath, head) {
  const list = Array.isArray(nodes) ? nodes.filter(n => n && n.id) : []
  const byID = new Map(list.map(n => [n.id, n]))
  const onPath = new Set(Array.isArray(currentPath) ? currentPath : [])
  const childrenOf = new Map()
  const roots = []
  for (const n of list) {
    const pid = n.parent_id && byID.has(n.parent_id) ? n.parent_id : null
    if (pid === null) { roots.push(n); continue }
    if (!childrenOf.has(pid)) childrenOf.set(pid, [])
    childrenOf.get(pid).push(n)
  }
  roots.sort(nodeOrd)
  for (const xs of childrenOf.values()) xs.sort(nodeOrd)

  const rows = []
  const seen = new Set()
  const visit = (node, level) => {
    if (seen.has(node.id)) return
    seen.add(node.id)
    const kids = childrenOf.get(node.id) || []
    rows.push({
      node,
      level,
      onPath: onPath.has(node.id),
      isCurrent: node.id === head,
      isFork: kids.length > 1,
    })
    if (!kids.length) return
    const primary = kids.find(k => onPath.has(k.id)) || kids[0]
    visit(primary, level)
    for (const k of kids) {
      if (k.id !== primary.id) visit(k, level + 1)
    }
  }
  for (const r of roots) visit(r, 0)
  // Orphans with dangling parent_id already treated as roots; nothing left unseen.
  return rows
}

// Converts rows into real parent-child edges for the SVG branch rail.
// Drawing edges from parent_id avoids implying a false chain when several siblings share a level.
export function buildWorldlineEdges(rows) {
  const list = Array.isArray(rows) ? rows : []
  const indexByID = new Map(list.map((row, index) => [row?.node?.id, index]))
  const edges = []
  list.forEach((row, childIndex) => {
    const parentIndex = indexByID.get(row?.node?.parent_id)
    if (parentIndex === undefined) return
    const parent = list[parentIndex]
    edges.push({
      id: `${parent.node.id}:${row.node.id}`,
      parentIndex,
      childIndex,
      parentLevel: parent.level,
      childLevel: row.level,
      onPath: Boolean(parent.onPath && row.onPath),
    })
  })
  return edges
}

export function worldlineMaxLevel(rows) {
  let max = 0
  for (const r of rows || []) if (r.level > max) max = r.level
  return max
}

// Version group lookup for a user message id; returns null when absent or single-version.
export function messageVersionInfo(worldline, userMessageID) {
  const groups = worldline?.message_versions
  if (!groups || !userMessageID) return null
  const g = groups[userMessageID]
  if (!g || !(g.total > 1)) return null
  return g
}

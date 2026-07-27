// worldlineTree.js — pure helpers for the persistent worldline branch panel.
// Level rule (project memory L15): nodes on one continuous chain share the same
// rail level; only a fork side-branch gains +1 level. Never indent per parent_id.

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
    let primary = kids.find(k => onPath.has(k.id)) || kids[0]
    visit(primary, level)
    for (const k of kids) {
      if (k.id !== primary.id) visit(k, level + 1)
    }
  }
  for (const r of roots) visit(r, 0)
  // Orphans with dangling parent_id already treated as roots; nothing left unseen.
  return rows
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

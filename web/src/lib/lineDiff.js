// Line-level diff for file_patch / file_write tool call rendering.
// Pure functions, no React, so they can be unit tested directly.

const MAX_DP_CELLS = 1200 * 1200 // guard against pathological O(n*m) blowups

function splitLines(text) {
  if (text == null) return []
  const s = String(text)
  if (s === '') return []
  return s.replace(/\r\n?/g, '\n').split('\n')
}

// Longest common subsequence table backtrack -> ordered ops.
function lcsOps(a, b) {
  const n = a.length
  const m = b.length
  const dp = new Uint32Array((n + 1) * (m + 1))
  const w = m + 1
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i * w + j] = a[i] === b[j]
        ? dp[(i + 1) * w + (j + 1)] + 1
        : Math.max(dp[(i + 1) * w + j], dp[i * w + (j + 1)])
    }
  }
  const ops = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      ops.push({ type: 'ctx', text: a[i] })
      i++
      j++
    } else if (dp[(i + 1) * w + j] >= dp[i * w + (j + 1)]) {
      ops.push({ type: 'del', text: a[i] })
      i++
    } else {
      ops.push({ type: 'add', text: b[j] })
      j++
    }
  }
  while (i < n) ops.push({ type: 'del', text: a[i++] })
  while (j < m) ops.push({ type: 'add', text: b[j++] })
  return ops
}

// Cheap fallback when the DP table would be too large: trim the shared
// head/tail, then treat the middle as a wholesale replacement.
function coarseOps(a, b) {
  let head = 0
  while (head < a.length && head < b.length && a[head] === b[head]) head++
  let tail = 0
  while (
    tail < a.length - head &&
    tail < b.length - head &&
    a[a.length - 1 - tail] === b[b.length - 1 - tail]
  ) tail++
  const ops = []
  for (let k = 0; k < head; k++) ops.push({ type: 'ctx', text: a[k] })
  for (let k = head; k < a.length - tail; k++) ops.push({ type: 'del', text: a[k] })
  for (let k = head; k < b.length - tail; k++) ops.push({ type: 'add', text: b[k] })
  for (let k = a.length - tail; k < a.length; k++) ops.push({ type: 'ctx', text: a[k] })
  return ops
}

/**
 * Build a unified-diff style row list.
 * @param {string} oldText
 * @param {string} newText
 * @param {{context?: number}} [opts] context = unchanged lines kept around a change
 * @returns {{rows: Array, added: number, removed: number, truncated: boolean}}
 *   row: { type: 'ctx'|'add'|'del', text, oldNo, newNo } | { type: 'gap', count }
 */
export function computeLineDiff(oldText, newText, opts = {}) {
  const context = Number.isFinite(opts.context) ? Math.max(0, opts.context) : 3
  const a = splitLines(oldText)
  const b = splitLines(newText)
  const truncated = (a.length + 1) * (b.length + 1) > MAX_DP_CELLS
  const ops = truncated ? coarseOps(a, b) : lcsOps(a, b)

  let oldNo = 0
  let newNo = 0
  let added = 0
  let removed = 0
  const numbered = ops.map(op => {
    if (op.type === 'ctx') {
      oldNo++
      newNo++
      return { type: 'ctx', text: op.text, oldNo, newNo }
    }
    if (op.type === 'del') {
      oldNo++
      removed++
      return { type: 'del', text: op.text, oldNo, newNo: null }
    }
    newNo++
    added++
    return { type: 'add', text: op.text, oldNo: null, newNo }
  })

  const keep = new Array(numbered.length).fill(false)
  for (let k = 0; k < numbered.length; k++) {
    if (numbered[k].type === 'ctx') continue
    for (let d = -context; d <= context; d++) {
      const idx = k + d
      if (idx >= 0 && idx < numbered.length) keep[idx] = true
    }
  }
  const anyChange = numbered.some(r => r.type !== 'ctx')
  if (!anyChange) keep.fill(true)

  const rows = []
  let skipped = 0
  for (let k = 0; k < numbered.length; k++) {
    if (keep[k]) {
      if (skipped > 0) {
        rows.push({ type: 'gap', count: skipped })
        skipped = 0
      }
      rows.push(numbered[k])
    } else {
      skipped++
    }
  }
  if (skipped > 0) rows.push({ type: 'gap', count: skipped })

  return { rows, added, removed, truncated }
}

/** Rows for a whole-file write: every line counts as an addition. */
export function computeWriteRows(text) {
  const lines = splitLines(text)
  return {
    rows: lines.map((t, i) => ({ type: 'add', text: t, oldNo: null, newNo: i + 1 })),
    added: lines.length,
    removed: 0,
    truncated: false,
  }
}

export const __test = { splitLines, lcsOps, coarseOps }

export const MARKDOWN_CHAR_LIMIT = 70000
export const MARKDOWN_LINE_LIMIT = 1200
export const MARKDOWN_BLOCK_LIMIT = 360
export const LIST_ITEM_LIMIT = 240
export const LONG_TEXT_PREVIEW_CHARS = 18000
export const JSON_TREE_CHILD_LIMIT = 160
export const JSON_TREE_STRING_LIMIT = 1400


export const FINAL_MARKER_RE = /^```+\s*\n?\[Info\]\s*Final response to user\.\s*\n?```+\s*$/i
export const TURN_HEADER_RE = /^\s*(?:\*\*)?\s*LLM Running\s*\(Turn\s+(\d+)\)\s*(?:\.\.\.)?\s*(?:\*\*)?\s*$/i
const FENCE_LINE_RE = /^\s*(```+|~~~+)/
const FINAL_INFO_LINE_RE = /^\s*\[Info\]\s*Final response to user\.\s*$/i
const FINAL_OPEN_FENCE_RE = /^\s*```+\s*$/
const FINAL_INLINE_RE = /^\s*```+\s*\[Info\]\s*Final response to user\.\s*```+\s*$/i

export const cleanAssistantRunBody = (s = '') => String(s || '')
  .replace(/<summary>[\s\S]*?<\/summary>/gi, '')
  .replace(/\n{3,}/g, '\n\n')
  .trim()

const findTopLevelAssistantMarkers = (full = '') => {
  const markers = []
  const lines = String(full || '').split('\n')
  const offsets = []
  let offset = 0
  for (const line of lines) {
    offsets.push(offset)
    offset += line.length + 1
  }

  let fence = null
  for (let i = 0; i < lines.length; i += 1) {
    const line = lines[i]
    const lineStart = offsets[i]
    const lineEnd = lineStart + line.length

    if (!fence) {
      const turnMatch = line.match(TURN_HEADER_RE)
      if (turnMatch) {
        markers.push({ type: 'turn', turn: Number(turnMatch[1]) || markers.length + 1, index: lineStart, end: lineEnd })
        continue
      }
      if (line.match(FINAL_INLINE_RE)) {
        markers.push({ type: 'final', index: lineStart, end: lineEnd })
        continue
      }
      if (line.match(FINAL_OPEN_FENCE_RE) && i + 2 < lines.length && lines[i + 1].match(FINAL_INFO_LINE_RE) && lines[i + 2].match(FINAL_OPEN_FENCE_RE)) {
        markers.push({ type: 'final', index: lineStart, end: offsets[i + 2] + lines[i + 2].length })
        i += 2
        continue
      }
    }

    const fenceMatch = line.match(FENCE_LINE_RE)
    if (fenceMatch) {
      const ticks = fenceMatch[1]
      if (!fence) {
        fence = ticks[0]
      } else if (ticks[0] === fence) {
        fence = null
      }
    }
  }
  return markers
}

export const parseAssistantContent = (raw = '') => {
  const full = String(raw || '').replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const markers = findTopLevelAssistantMarkers(full)
  const finalMarker = markers.find((m) => m.type === 'final')
  const turnMarkers = markers.filter((m) => m.type === 'turn' && (!finalMarker || m.index < finalMarker.index))
  const processEnd = finalMarker ? finalMarker.index : full.length
  const finalText = finalMarker ? full.slice(finalMarker.end) : ''
  const runs = []

  if (turnMarkers.length) {
    turnMarkers.forEach((m, i) => {
      const start = m.end
      const end = i + 1 < turnMarkers.length ? turnMarkers[i + 1].index : processEnd
      const chunk = full.slice(start, end).trim()
      const summary = chunk.match(/<summary>([\s\S]*?)<\/summary>/i)
      const title = summary?.[1]?.trim() || `Turn ${m.turn}`
      runs.push({ turn: m.turn, title, body: cleanAssistantRunBody(chunk) })
    })
    return { runs, body: (finalText || '').replace(/\n{3,}/g, '\n\n').trim() }
  }

  return { runs: [], body: full.replace(/^```+\s*\n?\[Info\]\s*Final response to user\.\s*\n?```+\s*$/gim, '').replace(/\n{3,}/g, '\n\n').trim() }
}

const isBlankLine = (line = '') => /^[\t\f\v ]*$/.test(line)

export const textRenderStats = (text = '') => {
  const src = String(text || '')
  const normalized = src.replace(/\r\n/g, '\n').replace(/\r/g, '\n')
  const parts = normalized.length ? normalized.split('\n') : []
  let standaloneNewlineLines = 0
  for (const line of parts) {
    if (isBlankLine(line)) standaloneNewlineLines += 1
  }
  const lines = parts.length
  const lineGuard = lines > MARKDOWN_LINE_LIMIT && standaloneNewlineLines > MARKDOWN_LINE_LIMIT
  return {
    chars: src.length,
    lines,
    linesLabel: String(lines),
    standaloneNewlineLines,
    tooLarge: src.length > MARKDOWN_CHAR_LIMIT || lineGuard,
  }
}

export const previewLongText = (text = '', limit = LONG_TEXT_PREVIEW_CHARS) => {
  const src = String(text || '')
  let head = src.slice(0, limit).replace(/\r\n/g, '\n').replace(/\n{8,}/g, '\n\n\u2026 \u8fde\u7eed\u7a7a\u884c\u5df2\u6298\u53e0 \u2026\n\n')
  if (src.length > limit) head += `\n\n\u2026 \u5df2\u622a\u65ad\u9884\u89c8\uff0c\u5b8c\u6574\u5185\u5bb9 ${src.length.toLocaleString()} \u5b57\u7b26\uff0c\u53ef\u590d\u5236\u5168\u6587\u3002`
  return head
}

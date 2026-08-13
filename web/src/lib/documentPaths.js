const DOCUMENT_EXTENSION_PATTERN = /\.(?:md|markdown|mdown|mkdn|txt|text|rst|adoc|json|ya?ml|toml|ini|cfg|conf|log|csv|tsv|html?|xml|docx?|odt|pdf)(?:[?#][^\s"'`<>|]*)?$/i

// Keep path extraction opt-in. A tool log can contain many unrelated paths;
// only standalone paths in result/code blocks or paths on an explicit result
// label should become user-facing document previews.
const localDocumentPathPattern = /(?<![\w/:])((?:[a-z]:[\\/](?!\/)|\\\\|\/(?!\/)|(?:\.\.?[\\/]))[^\r\n"'`<>|]*?\.(?:md|markdown|mdown|mkdn|txt|text|rst|adoc|json|ya?ml|toml|ini|cfg|conf|log|csv|tsv|html?|xml|docx?|odt|pdf)(?:[?#][^\s"'`<>|]*)?)/i
const resultLabelPattern = /(?:document|file|report|artifact|output|saved|created|completed|merged|result|evidence|proof|record|export|generated|文档|文件|报告|产物|输出|保存|生成|完成|合并|结果|证据|记录|导出)/iu

const cleanCandidate = (value = '') => String(value || '')
  .trim()
  .replace(/^[`'"([{]+|[`'"\])}>.,;:!?，。；：、]+$/g, '')
  .trim()

const isLocalDocumentPath = (value = '') => {
  const path = cleanCandidate(value)
  if (!path || /^https?:\/\//i.test(path) || /^file:\/\//i.test(path) || /^[a-z]:\/\//i.test(path) || /^data:/i.test(path) || path.startsWith('//')) return false
  if (/^\/(?:sdcard|storage|mnt|data)\//i.test(path) || /[\\/]chat_uploads[\\/]/i.test(path)) return false
  if (!DOCUMENT_EXTENSION_PATTERN.test(path)) return false
  return /^[a-z]:[\\/]/i.test(path) || /^\\\\/.test(path) || /^\/(?!\/)/.test(path) || /^\.\.?[\\/]/.test(path)
}

const collectAllPaths = (line, candidates) => {
  const source = String(line || '')
  const matcher = new RegExp(localDocumentPathPattern.source, 'gi')
  for (const match of source.matchAll(matcher)) {
    if (isLocalDocumentPath(match[1])) candidates.push(match[1])
  }
}

const collectStandalonePath = (line, candidates) => {
  const original = String(line || '').trim()
  const stripped = original.replace(/^(?:[-*+]\s+|\d+[.)]\s+)/, '')
  const match = stripped.match(localDocumentPathPattern)
  if (!match) return false
  const before = stripped.slice(0, match.index).trim()
  const after = stripped.slice((match.index || 0) + match[0].length).trim()
  const allowedPrefix = !before || /^(?:path|file|document|report|output|artifact|文档|文件|报告|路径)\s*[:：=]$/iu.test(before)
  if (!allowedPrefix || after) return false
  if (isLocalDocumentPath(match[1])) candidates.push(match[1])
  return true
}

const stripFileMarkers = (value = '') => String(value || '').replace(/\[FILE:[^\]\r\n]+\]/gi, '')

export const isDocumentPath = (value = '') => isLocalDocumentPath(value)

export const documentPathName = (value = '') => cleanCandidate(value).split(/[\\/]/).filter(Boolean).pop() || cleanCandidate(value) || 'document'

export const isMarkdownDocumentPath = (value = '') => /\.(?:md|markdown|mdown|mkdn)$/i.test(cleanCandidate(value).split(/[?#]/)[0])

export const extractDocumentPaths = (text = '') => {
  const source = stripFileMarkers(String(text || '').replace(/\r\n/g, '\n'))
  const candidates = []
  const fencePattern = /```[^\n]*\n([\s\S]*?)```/g
  for (const match of source.matchAll(fencePattern)) {
    for (const line of match[1].split('\n')) collectStandalonePath(line, candidates)
  }

  const withoutFences = source.replace(fencePattern, '')
  let waitingForPath = false
  for (const line of withoutFences.split('\n')) {
    const trimmed = line.trim()
    const labeled = resultLabelPattern.test(line)
    const match = line.match(localDocumentPathPattern)
    if (labeled && match) {
      collectAllPaths(line, candidates)
      waitingForPath = false
      continue
    }
    if (waitingForPath && match) {
      collectAllPaths(line, candidates)
      waitingForPath = false
      continue
    }
    if (labeled && /[:：=]\s*$/.test(trimmed)) waitingForPath = true
    else if (trimmed) waitingForPath = false
  }

  const seen = new Set()
  return candidates.map(cleanCandidate).filter(path => {
    if (!isLocalDocumentPath(path)) return false
    const key = path.replace(/\\/g, '/').toLowerCase()
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

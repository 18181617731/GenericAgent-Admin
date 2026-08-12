const IMAGE_EXTENSION = String.raw`(?:png|jpe?g|gif|webp|bmp|svg)`

const cleanCandidate = (value = '') => {
  let path = String(value || '').trim()
  path = path.replace(/^file:\/\//i, '')
  path = path.replace(/^[`'"<([{]+|[`'">)\]},;:]+$/g, '').trim()
  return path
}

const isLocalImagePath = (value = '') => {
  const path = cleanCandidate(value)
  if (!path || /^https?:\/\//i.test(path) || /^[a-z]:\/\//i.test(path) || /^data:/i.test(path) || path.startsWith('//')) return false
  if (/^\/(?:sdcard|storage|mnt|data)\//i.test(path) || /[\\/]chat_uploads[\\/]/i.test(path)) return false
  if (!new RegExp(`\\.${IMAGE_EXTENSION}(?:[?#][^\\s]*)?$`, 'i').test(path)) return false
  return /^[a-z]:[\\/]/i.test(path) || /^\/(?!\/)/.test(path) || /^(?:\.?[\\/])?(?:temp|output|outputs|artifacts?|generated_images)[\\/]/i.test(path)
}

const localImagePathPattern = /(?<![\w/:])((?:[a-z]:[\\/](?!\/)|\/(?!\/)|(?:\.?[\\/])?(?:temp|output|outputs|artifacts?|generated_images)[\\/])[^\r\n"'`<>|]*?\.(?:png|jpe?g|gif|webp|bmp|svg)(?:[?#][^\s"'`<>|]*)?)/i
// Result messages often label a screenshot with a natural-language line such as
// "Screenshot: C:\\...\\evidence.png" instead of emitting [FILE:...]. Keep
// extraction opt-in, but include common evidence/result labels in both languages.
const generatedImageLabelPattern = /(?:generated|saved|image|picture|photo|screenshot|screen\s*shot|screen\s*capture|preview|output|artifact|evidence|proof|result|\u751f\u6210|\u56fe\u7247|\u56fe\u50cf|\u7167\u7247|\u622a\u56fe|\u622a\u5c4f|\u5c4f\u5e55\u622a\u56fe|\u4fdd\u5b58|\u8f93\u51fa|\u6210\u679c|\u8bc1\u636e|\u590d\u6838|\u8bb0\u5f55|\u7ed3\u679c)/iu

const stripCodeBlocks = (value = '') => String(value || '').replace(/`{3,}[^\r\n]*\r?\n[\s\S]*?`{3,}/g, '')

const collectLabeledImagePaths = (source, candidates) => {
  let waitingForPath = false
  for (const line of source.split(/\r?\n/)) {
    const labeled = generatedImageLabelPattern.test(line)
    const blank = !line.trim()
    if (!labeled && !waitingForPath && !blank) continue
    const match = line.match(localImagePathPattern)
    if (match) {
      candidates.push(match[1])
      waitingForPath = false
      continue
    }
    // Preserve a heading's pending path across formatting blank lines, but do
    // not let an unrelated following paragraph inherit the label forever.
    if (labeled) waitingForPath = /[:：]\s*$/.test(line)
    else if (!blank) waitingForPath = false
  }
}

export const extractGeneratedImagePaths = (text = '') => {
  const source = stripCodeBlocks(text)
  const candidates = []
  const collect = (re) => {
    for (const match of source.matchAll(re)) candidates.push(match[1])
  }

  collect(new RegExp(String.raw`!\[[^\]]*\]\(([^)\r\n]+?\.${IMAGE_EXTENSION}(?:[?#][^\s)]*)?)\)`, 'gi'))
  collect(new RegExp(String.raw`\[FILE:([^\]\r\n]+?\.${IMAGE_EXTENSION}(?:[?#][^\s\]]*)?)\]`, 'gi'))
  collect(new RegExp(String.raw`<img[^>]+src=["']([^"'\r\n]+?\.${IMAGE_EXTENSION}(?:[?#][^\s"']*)?)["']`, 'gi'))
  collectLabeledImagePaths(source, candidates)

  const seen = new Set()
  return candidates.map(cleanCandidate).filter(path => {
    if (!isLocalImagePath(path)) return false
    const key = path.replace(/\\/g, '/').toLowerCase()
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}

export const generatedImageURL = (path = '') => `/api/files/image?path=${encodeURIComponent(String(path || ''))}`
export const generatedImageDownloadURL = (path = '') => `/api/files/download?path=${encodeURIComponent(String(path || ''))}`

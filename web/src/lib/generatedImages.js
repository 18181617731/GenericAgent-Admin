const IMAGE_EXTENSION = String.raw`(?:png|jpe?g|gif|webp|bmp)`

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

const localImagePathPattern = /(?<![\w/:])((?:[a-z]:[\\/](?!\/)|\/(?!\/)|(?:\.?[\\/])?(?:temp|output|outputs|artifacts?|generated_images)[\\/])[^\r\n"'`<>|]*?\.(?:png|jpe?g|gif|webp|bmp)(?:[?#][^\s"'`<>|]*)?)/i
const generatedImageLabelPattern = /(?:generated|saved|image|picture|photo|output|artifact|生成|图片|图像|照片|保存|输出|成果)/iu

const stripCodeBlocks = (value = '') => String(value || '').replace(/`{3,}[^\r\n]*\r?\n[\s\S]*?`{3,}/g, '')

const collectLabeledImagePaths = (source, candidates) => {
  let waitingForPath = false
  for (const line of source.split(/\r?\n/)) {
    const labeled = generatedImageLabelPattern.test(line)
    if (!labeled && !waitingForPath) continue
    const match = line.match(localImagePathPattern)
    if (match) {
      candidates.push(match[1])
      waitingForPath = false
      continue
    }
    waitingForPath = labeled && /[:：]\s*$/.test(line)
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

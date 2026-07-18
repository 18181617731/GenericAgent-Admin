const IMAGE_EXTENSION = String.raw`(?:png|jpe?g|gif|webp|bmp)`

const cleanCandidate = (value = '') => {
  let path = String(value || '').trim()
  path = path.replace(/^file:\/\//i, '')
  path = path.replace(/^[`'"<([{]+|[`'">)\]},;:]+$/g, '').trim()
  return path
}

const isLocalImagePath = (value = '') => {
  const path = cleanCandidate(value)
  if (!path || /^https?:\/\//i.test(path) || /^data:/i.test(path) || path.startsWith('//')) return false
  if (!new RegExp(`\\.${IMAGE_EXTENSION}(?:[?#][^\\s]*)?$`, 'i').test(path)) return false
  return /^[a-z]:[\\/]/i.test(path) || /^\/(?!\/)/.test(path) || /^(?:\.?[\\/])?(?:temp|output|outputs|artifacts?|generated_images)[\\/]/i.test(path)
}

export const extractGeneratedImagePaths = (text = '') => {
  const source = String(text || '')
  const candidates = []
  const collect = (re) => {
    for (const match of source.matchAll(re)) candidates.push(match[1])
  }

  collect(new RegExp(String.raw`!\[[^\]]*\]\(([^)\r\n]+?\.${IMAGE_EXTENSION}(?:[?#][^\s)]*)?)\)`, 'gi'))
  collect(new RegExp(String.raw`\[FILE:([^\]\r\n]+?\.${IMAGE_EXTENSION}(?:[?#][^\s\]]*)?)\]`, 'gi'))
  collect(new RegExp(String.raw`["'\x60]([^"'\x60\r\n]+?\.${IMAGE_EXTENSION}(?:[?#][^\s"'\x60]*)?)["'\x60]`, 'gi'))
  collect(new RegExp(String.raw`\b([a-z]:[\\/][^\r\n"'\x60<>|]*?\.${IMAGE_EXTENSION}(?:[?#][^\s"'\x60<>|]*)?)`, 'gi'))
  collect(new RegExp(String.raw`(?:^|[\s(])(/(?!/)[^\r\n"'\x60<>|]*?\.${IMAGE_EXTENSION}(?:[?#][^\s"'\x60<>|]*)?)`, 'gim'))
  collect(new RegExp(String.raw`(?:^|\r?\n)[ \t]*((?:\.?[\\/])?(?:temp|output|outputs|artifacts?|generated_images)[\\/][^\r\n"'\x60<>|]*?\.${IMAGE_EXTENSION}(?:[?#][^\s"'\x60<>|]*)?)`, 'gim'))

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

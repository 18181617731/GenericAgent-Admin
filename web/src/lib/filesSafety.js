export const clampTailLines = (value, min = 1, max = 5000) => {
  const n = Number(value)
  if (!Number.isFinite(n)) return min
  return Math.min(max, Math.max(min, Math.trunc(n)))
}

export const dirnameForPath = (path = '') => {
  const normalized = String(path || '').replaceAll('\\', '/')
  return normalized.includes('/') ? normalized.split('/').slice(0, -1).join('/') : ''
}

const normalizedEditorText = value => String(value || '').replaceAll('\r\n', '\n').replaceAll('\r', '\n')

export const fileEditorDirty = (content = '', loadedContent = '') => normalizedEditorText(content) !== normalizedEditorText(loadedContent)

export const saveReviewText = ({ path = '', loadedPath = '', dirty = false } = {}) => {
  if (!path) return 'Choose a file before saving.'
  if (loadedPath && path !== loadedPath) return `Review target: saving editor content loaded from ${loadedPath} to ${path}.`
  return dirty ? `Review target: saving changes to ${path}.` : `No unsaved changes for ${path}.`
}

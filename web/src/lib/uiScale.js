export const UI_SCALE_STORAGE_KEY = 'ga-admin-ui-scale'
export const DEFAULT_UI_SCALE = 1
export const UI_SCALE_MIN = 0.8
export const UI_SCALE_MAX = 1.2
export const UI_SCALE_STEP = 0.05

const precision = value => Math.round(value * 100) / 100

export const normalizeUIScale = value => {
  const parsed = Number(value)
  if (!Number.isFinite(parsed)) return DEFAULT_UI_SCALE
  return precision(Math.min(UI_SCALE_MAX, Math.max(UI_SCALE_MIN, parsed)))
}

export const formatUIScale = value => `${Math.round(normalizeUIScale(value) * 100)}%`

export const stepUIScale = (value, direction) => normalizeUIScale(
  normalizeUIScale(value) + (direction < 0 ? -UI_SCALE_STEP : UI_SCALE_STEP),
)

export const getInitialUIScale = (storage = typeof window !== 'undefined' ? window.localStorage : null) => {
  const stored = storage?.getItem?.(UI_SCALE_STORAGE_KEY)
  return stored == null ? DEFAULT_UI_SCALE : normalizeUIScale(stored)
}

export const applyUIScaleToDocument = (value, documentRef = globalThis.document) => {
  const normalized = normalizeUIScale(value)
  const root = documentRef?.documentElement
  if (root) {
    root.dataset.uiScale = String(Math.round(normalized * 100))
    root.style.setProperty('--ga-ui-scale', String(normalized))
    root.style.setProperty('--ga-ui-scale-width', String(1 / normalized))
  }
  return normalized
}

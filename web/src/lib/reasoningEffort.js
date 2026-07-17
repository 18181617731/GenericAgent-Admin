export const REASONING_EFFORT_LEVELS = Object.freeze([
  'off',
  'none',
  'minimal',
  'low',
  'medium',
  'high',
  'xhigh',
  'max',
])

const REASONING_EFFORT_LEVEL_SET = new Set(REASONING_EFFORT_LEVELS)

export const normalizeReasoningEffort = (value) => {
  const normalized = String(value || '').trim().toLowerCase()
  return REASONING_EFFORT_LEVEL_SET.has(normalized) ? normalized : 'off'
}

const LABELS = {
  off: '默认',
  none: 'None',
  minimal: 'Minimal',
  low: 'Low',
  medium: 'Medium',
  high: 'High',
  xhigh: 'XHigh',
  max: 'Max',
}

export const REASONING_EFFORT_OPTIONS = Object.freeze(
  REASONING_EFFORT_LEVELS.map(value => Object.freeze({ value, label: LABELS[value] })),
)

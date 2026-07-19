const clean = value => String(value ?? '').trim()

export const normalizePromptPresets = value => {
  if (!Array.isArray(value)) return []
  const seen = new Set()
  const result = []
  for (const item of value) {
    const preset = {
      id: clean(item?.id),
      name: clean(item?.name),
      content: clean(item?.content),
    }
    if (!preset.id || seen.has(preset.id)) continue
    seen.add(preset.id)
    result.push(preset)
  }
  return result
}

export const createPromptPreset = (existing = [], seed = '') => {
  const ids = new Set(normalizePromptPresets(existing).map(item => item.id))
  const base = clean(seed) || `prompt-${Date.now().toString(36)}`
  let id = base
  let suffix = 2
  while (ids.has(id)) id = `${base}-${suffix++}`
  return { id, name: '', content: '' }
}

export const promptPresetPatch = value => ({
  extra_sys_prompt_preset_id: clean(value),
})

export const selectedPromptPresetView = ({ presets, selectedID, snapshot = [] } = {}) => {
  const id = clean(selectedID)
  if (!id) return { id: '', name: '不使用预设', content: '', orphaned: false }
  const item = normalizePromptPresets(presets).find(preset => preset.id === id)
  if (item) return { ...item, orphaned: false }
  const content = Array.isArray(snapshot) ? clean(snapshot[0]) : ''
  return { id, name: '已删除的预设', content, orphaned: true }
}

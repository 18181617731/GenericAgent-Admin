import test from 'node:test'
import assert from 'node:assert/strict'

import {
  createPromptPreset,
  normalizePromptPresets,
  promptPresetPatch,
  selectedPromptPresetView,
} from './promptPresets.js'

test('prompt presets normalize API data and keep stable unique IDs', () => {
  assert.deepEqual(normalizePromptPresets([
    { id: ' alpha ', name: ' Alpha ', content: ' First ' },
    { id: 'alpha', name: 'duplicate', content: 'ignored' },
    { id: '', name: 'invalid', content: 'invalid' },
  ]), [{ id: 'alpha', name: 'Alpha', content: 'First' }])
  assert.equal(createPromptPreset([{ id: 'prompt-seed', name: 'A', content: 'A' }], 'prompt-seed').id, 'prompt-seed-2')
})

test('prompt preset patch expresses one selection or explicit clear', () => {
  assert.deepEqual(promptPresetPatch(' alpha '), { extra_sys_prompt_preset_id: 'alpha' })
  assert.deepEqual(promptPresetPatch(''), { extra_sys_prompt_preset_id: '' })
})

test('selected prompt preset view preserves deleted preset snapshot', () => {
  assert.deepEqual(selectedPromptPresetView({
    presets: [],
    selectedID: 'removed',
    snapshot: [' Historical instructions '],
  }), {
    id: 'removed',
    name: '已删除的预设',
    content: 'Historical instructions',
    orphaned: true,
  })
})

import test from 'node:test'
import assert from 'node:assert/strict'
import { groupRuntimeModels, modelProvider, runtimeModelLabel } from './format.js'

test('groups runtime models by explicit provider and keeps numeric indexes', () => {
  const groups = groupRuntimeModels([
    { index: 4, provider: 'OpenAI', model: 'gpt-5' },
    { index: 7, provider: 'Anthropic', model: 'claude-sonnet-4' },
    { index: 9, provider: 'OpenAI', model: 'gpt-5-mini' },
  ])
  assert.deepEqual(groups.map(group => [group.value, group.models.map(model => model.value)]), [
    ['OpenAI', [4, 9]],
    ['Anthropic', [7]],
  ])
})

test('derives provider and model labels from legacy runtime names', () => {
  const model = { index: 2, name: 'Azure/gpt-4.1', model: 'gpt-4.1' }
  assert.equal(modelProvider(model), 'Azure')
  assert.equal(runtimeModelLabel(model), 'gpt-4.1')
  assert.equal(modelProvider({ index: 3, name: 'local/llama' }), 'local')
  assert.equal(runtimeModelLabel({ index: 3, name: 'local/llama' }), 'llama')
})

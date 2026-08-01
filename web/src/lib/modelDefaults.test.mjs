import test from 'node:test'
import assert from 'node:assert/strict'
import { firstRuntimeModel, firstRuntimeModelNo, modelDisplayName, orderedRuntimeModels, runtimeModelDescription } from './modelDefaults.js'

test('orders runtime models by numeric index and keeps stable ties', () => {
  const models = [
    { index: 7, model: 'late' },
    { index: '2', model: 'first' },
    { index: 2, model: 'tie' },
    { model: 'unindexed' },
  ]

  assert.deepEqual(orderedRuntimeModels(models).map(model => model.model), ['first', 'tie', 'late', 'unindexed'])
  assert.equal(firstRuntimeModel(models).model, 'first')
  assert.equal(firstRuntimeModelNo(models), 2)
})

test('falls back safely when no runtime model has a valid index', () => {
  assert.equal(firstRuntimeModelNo([{ index: 'invalid', model: 'broken' }], 9), 9)
  assert.equal(firstRuntimeModelNo([], 'invalid'), 0)
})

test('uses configured display names and provider details in runtime labels', () => {
  const model = { index: 4, provider: '自费帅 API', model: '4', display_name: '自费帅主模型' }
  assert.equal(modelDisplayName(model), '自费帅主模型')
  assert.equal(runtimeModelDescription(model), '自费帅 API · 自费帅主模型 · #4')
})

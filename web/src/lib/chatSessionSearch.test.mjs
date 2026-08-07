import test from 'node:test'
import assert from 'node:assert/strict'
import { clearSessionSearchHistory, loadSessionSearchHistory, normalizeSessionSearchHistory, saveSessionSearchHistory } from './chatSessionSearch.js'

const storage = () => {
  const values = new Map()
  return {
    getItem: key => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
  }
}

test('session search history normalizes scopes and removes duplicates', () => {
  assert.deepEqual(normalizeSessionSearchHistory([{ query:'模型', scope:'content' }, '模型', { query:'模型', scope:'content' }, { query:'  ' }]), [
    { query:'模型', scope:'content' },
    { query:'模型', scope:'all' },
  ])
})

test('session search history persists newest entries and clears safely', () => {
  const target = storage()
  assert.deepEqual(loadSessionSearchHistory(target), [])
  saveSessionSearchHistory({ query:'第一次', scope:'title' }, target)
  saveSessionSearchHistory({ query:'第二次', scope:'content' }, target)
  assert.deepEqual(loadSessionSearchHistory(target), [
    { query:'第二次', scope:'content' },
    { query:'第一次', scope:'title' },
  ])
  assert.deepEqual(clearSessionSearchHistory(target), [])
  assert.deepEqual(loadSessionSearchHistory(target), [])
})

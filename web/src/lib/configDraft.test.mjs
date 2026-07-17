import test from 'node:test'
import assert from 'node:assert/strict'
import { configDraftDirty, configDraftSnapshot } from './configDraft.js'

test('configDraftSnapshot keeps only user-editable settings', () => {
  assert.deepEqual(configDraftSnapshot('D:/GA', { python_path: 'python.exe', port: 9999 }), {
    ga_root: 'D:/GA',
    python_path: 'python.exe',
    chat_data_dir: '',
    proxy_mode: 'off',
    http_proxy: '',
    https_proxy: '',
    all_proxy: '',
    no_proxy: '',
  })
})

test('configDraftDirty detects edits and ignores runtime-only fields', () => {
  const saved = { ga_root: 'D:/GA', python_path: '', proxy_mode: 'off', effective_python: 'python.exe' }
  assert.equal(configDraftDirty('D:/GA', { ...saved, effective_python: 'other.exe' }, saved), false)
  assert.equal(configDraftDirty('D:/GA', { ...saved, proxy_mode: 'system' }, saved), true)
  assert.equal(configDraftDirty('D:/Other', saved, saved), true)
})

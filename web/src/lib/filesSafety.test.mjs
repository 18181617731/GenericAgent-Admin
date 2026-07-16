import test from 'node:test'
import assert from 'node:assert/strict'
import { clampTailLines, dirnameForPath, fileEditorDirty, saveReviewText } from './filesSafety.js'

test('clampTailLines keeps tail requests in a safe integer range', () => {
  assert.equal(clampTailLines('0'), 1)
  assert.equal(clampTailLines('12.9'), 12)
  assert.equal(clampTailLines('999999'), 5000)
  assert.equal(clampTailLines('bad'), 1)
})

test('dirnameForPath handles root, nested, and windows separators', () => {
  assert.equal(dirnameForPath('file.txt'), '')
  assert.equal(dirnameForPath('memory/global_mem.txt'), 'memory')
  assert.equal(dirnameForPath('memory\\sop\\x.md'), 'memory/sop')
})

test('fileEditorDirty and saveReviewText expose save-safety states', () => {
  assert.equal(fileEditorDirty('new', 'old'), true)
  assert.equal(fileEditorDirty('first\nsecond\n', 'first\r\nsecond\r\n'), false)
  assert.match(saveReviewText({ path: 'a.txt', loadedPath: 'b.txt', dirty: true }), /loaded from b\.txt to a\.txt/)
  assert.match(saveReviewText({ path: 'a.txt', loadedPath: 'a.txt', dirty: true }), /saving changes to a\.txt/)
  assert.match(saveReviewText({ path: '', loadedPath: 'a.txt', dirty: true }), /Choose a file/)
})

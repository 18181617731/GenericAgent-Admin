import test from 'node:test'
import assert from 'node:assert/strict'
import { confirmDanger, isDangerDisabled } from './danger.js'

test('confirmDanger prefixes operation and returns confirm result', () => {
  const seen = []
  const ok = confirmDanger('files-write', 'Write file?', (msg) => { seen.push(msg); return true })
  assert.equal(ok, true)
  assert.equal(seen[0], '[files-write] Write file?')
})

test('confirmDanger is safe when confirm is unavailable', () => {
  assert.equal(confirmDanger('x', 'y', undefined), false)
})

test('isDangerDisabled folds busy or invalid states', () => {
  assert.equal(isDangerDisabled(false, '', 0), false)
  assert.equal(isDangerDisabled(false, true), true)
})

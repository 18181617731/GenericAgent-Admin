import { strict as assert } from 'node:assert'
import test from 'node:test'
import { lastUserMessageID, nextActiveSession } from './chatSessionActions.js'

test('lastUserMessageID returns the latest user message with an id', () => {
  assert.equal(lastUserMessageID([{ role: 'user', id: 'u1' }, { role: 'assistant', id: 'a1' }, { role: 'user', id: 'u2' }]), 'u2')
  assert.equal(lastUserMessageID([{ role: 'assistant', id: 'a1' }]), '')
})

test('nextActiveSession skips the archived or excluded session', () => {
  assert.equal(nextActiveSession([{ id: 'one' }, { id: 'two', archived: true }, { id: 'three' }], 'one'), 'three')
})

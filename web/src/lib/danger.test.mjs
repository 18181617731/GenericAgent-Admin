import test from 'node:test'
import assert from 'node:assert/strict'
import {
  confirmDanger,
  isDangerDisabled,
  registerDialogAdapter,
  showAppAlert,
} from './danger.js'

const withAdapter = async (adapter, run) => {
  const unregister = registerDialogAdapter(adapter)
  try {
    return await run()
  } finally {
    unregister()
  }
}

test('confirmDanger sends a structured request and returns the adapter result', async () => {
  const seen = []
  const ok = await withAdapter(request => {
    seen.push(request)
    return true
  }, () => confirmDanger('files-write', 'Write file?'))

  assert.equal(ok, true)
  assert.deepEqual(seen, [{
    kind: 'confirm',
    locale: 'zh',
    operation: 'files-write',
    message: 'Write file?',
  }])
})

test('confirmDanger preserves cancellation', async () => {
  const ok = await withAdapter(() => false, () => confirmDanger('files-delete', 'Delete file?'))
  assert.equal(ok, false)
})

test('requests wait for an app dialog host instead of falling back to a native dialog', async () => {
  let settled = false
  const pending = confirmDanger('queued-op', 'Queued?').then(value => {
    settled = true
    return value
  })

  await Promise.resolve()
  assert.equal(settled, false)

  const seen = []
  const unregister = registerDialogAdapter(request => {
    seen.push(request)
    return true
  })
  try {
    assert.equal(await pending, true)
    assert.equal(seen[0].operation, 'queued-op')
  } finally {
    unregister()
  }
})

test('adapter failures safely cancel confirmation', async () => {
  const rejected = await withAdapter(() => Promise.reject(new Error('dialog failed')), () => confirmDanger('x', 'y'))
  assert.equal(rejected, false)

  const thrown = await withAdapter(() => { throw new Error('dialog failed') }, () => confirmDanger('x', 'y'))
  assert.equal(thrown, false)
})

test('showAppAlert sends alert metadata through the same app host', async () => {
  const seen = []
  const result = await withAdapter(request => {
    seen.push(request)
    return true
  }, () => showAppAlert('Open failed', {
    title: 'Error',
    operation: 'chat-file-open',
    confirmLabel: 'Close',
  }))

  assert.equal(result, true)
  assert.deepEqual(seen[0], {
    kind: 'alert',
    locale: 'zh',
    message: 'Open failed',
    title: 'Error',
    operation: 'chat-file-open',
    confirmLabel: 'Close',
  })
})

test('an old unregister callback cannot remove a newer adapter', async () => {
  const oldAdapter = () => false
  const unregisterOld = registerDialogAdapter(oldAdapter)
  const unregisterNew = registerDialogAdapter(() => true)
  unregisterOld()
  try {
    assert.equal(await confirmDanger('x', 'y'), true)
  } finally {
    unregisterNew()
  }
})

test('isDangerDisabled folds busy or invalid states', () => {
  assert.equal(isDangerDisabled(false, '', 0), false)
  assert.equal(isDangerDisabled(false, true), true)
})

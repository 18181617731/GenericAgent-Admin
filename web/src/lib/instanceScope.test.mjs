import test from 'node:test'
import assert from 'node:assert/strict'
import { addInstanceToURL, getSelectedInstanceID, INSTANCE_STORAGE_KEY, setSelectedInstanceID } from './instanceScope.js'

test('instance URLs preserve existing query and hash while adding scope', () => {
  assert.equal(addInstanceToURL('/api/files/list?path=memory#preview', 'beta'), '/api/files/list?path=memory&instance_id=beta#preview')
  assert.equal(addInstanceToURL('/assets/app.js', 'beta'), '/assets/app.js')
  assert.equal(addInstanceToURL('/api/config', ''), '/api/config')
})
test('selected instance storage is normalized and removable', () => {
  const values = new Map()
  const storage = {
    getItem: key => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
    removeItem: key => values.delete(key),
  }
  assert.equal(setSelectedInstanceID(' beta ', { storage, dispatch: false }), 'beta')
  assert.equal(values.get(INSTANCE_STORAGE_KEY), 'beta')
  assert.equal(getSelectedInstanceID(storage), 'beta')
  setSelectedInstanceID('', { storage, dispatch: false })
  assert.equal(getSelectedInstanceID(storage), '')
})

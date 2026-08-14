import test from 'node:test'
import assert from 'node:assert/strict'
import {
  DEFAULT_UI_SCALE,
  UI_SCALE_STORAGE_KEY,
  applyUIScaleToDocument,
  formatUIScale,
  getInitialUIScale,
  normalizeUIScale,
  stepUIScale,
} from './uiScale.js'

test('normalizes interface scale into the supported range', () => {
  assert.equal(normalizeUIScale('0.73'), 0.8)
  assert.equal(normalizeUIScale('1.17'), 1.17)
  assert.equal(normalizeUIScale('1.4'), 1.25)
  assert.equal(normalizeUIScale('not-a-number'), DEFAULT_UI_SCALE)
  assert.equal(formatUIScale(1.1), '110%')
})

test('steps interface scale without crossing the supported range', () => {
  assert.equal(stepUIScale(1, -1), 0.95)
  assert.equal(stepUIScale(1, 1), 1.05)
  assert.equal(stepUIScale(0.8, -1), 0.8)
  assert.equal(stepUIScale(1.25, 1), 1.25)
})

test('loads and applies a persisted interface scale', () => {
  const storage = { getItem: key => key === UI_SCALE_STORAGE_KEY ? '1.15' : null }
  assert.equal(getInitialUIScale(storage), 1.15)

  const root = { dataset: {}, style: { setProperty: (key, value) => { root.style[key] = value } } }
  assert.equal(applyUIScaleToDocument(1.15, { documentElement: root }), 1.15)
  assert.equal(root.dataset.uiScale, '115')
  assert.equal(root.style['--ga-ui-scale'], '1.15')
  assert.equal(root.style['--ga-ui-scale-width'], String(1 / 1.15))
})

import test from 'node:test'
import assert from 'node:assert/strict'

import {
  DEFAULT_THEME_ID,
  THEMES,
  applyThemeToDocument,
  getInitialTheme,
  getNextThemeId,
  getTheme,
  isThemeId,
} from '../themes.js'

test('theme registry has unique IDs and complete appearance metadata', () => {
  assert.ok(THEMES.length >= 3)
  assert.equal(new Set(THEMES.map(theme => theme.id)).size, THEMES.length)
  assert.ok(isThemeId(DEFAULT_THEME_ID))

  for (const theme of THEMES) {
    assert.match(theme.id, /^[a-z][a-z0-9-]*$/)
    assert.ok(['light', 'dark'].includes(theme.colorScheme))
    assert.equal(typeof theme.icon, 'object')
    assert.equal(typeof theme.label?.en, 'string')
    assert.equal(typeof theme.label?.zh, 'string')
    assert.ok(['default', 'dark'].includes(theme.antdAlgorithm))
    assert.equal(typeof theme.antdToken, 'object')
  }
})

test('theme navigation and invalid values are derived from the registry', () => {
  assert.equal(getTheme('missing').id, DEFAULT_THEME_ID)

  THEMES.forEach((theme, index) => {
    assert.equal(getTheme(theme.id), theme)
    assert.equal(getNextThemeId(theme.id), THEMES[(index + 1) % THEMES.length].id)
  })

  assert.equal(getNextThemeId('missing'), THEMES[0].id)
})

test('applying a theme synchronizes palette and shared color-scheme attributes', () => {
  const documentRef = { documentElement: { dataset: {} } }
  const darkTheme = THEMES.find(theme => theme.colorScheme === 'dark')
  assert.ok(darkTheme)

  assert.equal(applyThemeToDocument(darkTheme.id, documentRef), darkTheme)
  assert.deepEqual(documentRef.documentElement.dataset, {
    theme: darkTheme.id,
    colorScheme: darkTheme.colorScheme,
  })

  const fallback = applyThemeToDocument('missing', documentRef)
  assert.equal(fallback.id, DEFAULT_THEME_ID)
  assert.equal(documentRef.documentElement.dataset.theme, DEFAULT_THEME_ID)
  assert.equal(documentRef.documentElement.dataset.colorScheme, fallback.colorScheme)
})

test('initial theme honors valid storage and otherwise uses the product default', () => {
  const previousWindow = globalThis.window

  try {
    globalThis.window = {
      localStorage: { getItem: () => 'warm' },
      matchMedia: () => { throw new Error('system preference must not override the product default') },
    }
    assert.equal(getInitialTheme(), 'warm')

    globalThis.window.localStorage.getItem = () => 'missing'
    assert.equal(getInitialTheme(), DEFAULT_THEME_ID)
  } finally {
    if (previousWindow === undefined) delete globalThis.window
    else globalThis.window = previousWindow
  }
})

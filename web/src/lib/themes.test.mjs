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
  persistTheme,
  persistThemeLocal,
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

    globalThis.window.__GA_UI_THEME__ = 'dark'
    globalThis.window.localStorage.getItem = () => 'light'
    assert.equal(getInitialTheme(), 'dark')

    globalThis.window.__GA_UI_THEME__ = 'not-a-theme'
    assert.equal(getInitialTheme(), 'light')
  } finally {
    if (previousWindow === undefined) delete globalThis.window
    else globalThis.window = previousWindow
  }
})

test('persistThemeLocal writes localStorage and emits theme-change', () => {
  const previousWindow = globalThis.window
  const stored = {}
  const events = []
  globalThis.window = {
    localStorage: {
      getItem: (key) => stored[key] ?? null,
      setItem: (key, value) => { stored[key] = value },
    },
    dispatchEvent: (event) => { events.push(event) },
  }
  try {
    const theme = persistThemeLocal('dark')
    assert.equal(theme.id, 'dark')
    assert.equal(stored['ga-admin-theme'], 'dark')
    assert.equal(events.length, 1)
    assert.equal(events[0].type, 'ga-admin-theme-change')
    assert.equal(events[0].detail, 'dark')
  } finally {
    if (previousWindow === undefined) delete globalThis.window
    else globalThis.window = previousWindow
  }
})

test('persistTheme PUTs when the injected theme differs and skips when it matches', async () => {
  const previousWindow = globalThis.window
  const previousFetch = globalThis.fetch
  const stored = {}
  const calls = []
  globalThis.window = {
    localStorage: {
      getItem: (key) => stored[key] ?? null,
      setItem: (key, value) => { stored[key] = String(value) },
    },
    dispatchEvent: () => true,
  }
  globalThis.fetch = async (url, init = {}) => {
    calls.push({ url, method: init.method, headers: init.headers, body: init.body })
    return { ok: true, status: 200, text: async () => '{}' }
  }
  try {
    persistTheme('dark')
    for (let i = 0; i < 50 && calls.length === 0; i += 1) {
      await new Promise((resolve) => setTimeout(resolve, 10))
    }
    assert.equal(stored['ga-admin-theme'], 'dark')
    assert.equal(globalThis.window.__GA_UI_THEME__, 'dark')
    assert.equal(calls.length, 1)
    assert.equal(calls[0].url, '/api/ui/theme')
    assert.equal(calls[0].method, 'PUT')
    assert.equal(calls[0].body, JSON.stringify({ theme: 'dark' }))
    assert.equal(calls[0].headers['X-GA-Confirm'], 'dangerous')

    persistTheme('dark')
    await new Promise((resolve) => setTimeout(resolve, 50))
    assert.equal(calls.length, 1)
  } finally {
    if (previousWindow === undefined) delete globalThis.window
    else globalThis.window = previousWindow
    if (previousFetch === undefined) delete globalThis.fetch
    else globalThis.fetch = previousFetch
  }
})

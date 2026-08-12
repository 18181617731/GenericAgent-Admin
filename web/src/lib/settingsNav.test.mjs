import test from 'node:test'
import assert from 'node:assert/strict'
import { NAV_ITEMS } from './routing.js'
import { SETTINGS_GROUPS, settingsNavGroupOf, settingsNavItems, unassignedNavItems } from './settingsNav.js'
import { I18N } from './i18n.js'

test('every route belongs to exactly one settings group', () => {
  const items = settingsNavItems()
  assert.deepEqual(unassignedNavItems(), [])
  assert.equal(items.length, new Set(items).size, 'a route may not appear in two groups')
  assert.deepEqual([...items].sort(), [...NAV_ITEMS].sort())
})

test('groups and routes are labelled in both languages', () => {
  for (const lang of ['zh', 'en']) {
    const t = I18N[lang]
    for (const group of SETTINGS_GROUPS) {
      assert.ok(t.navGroups[group.id], `${lang} is missing a label for group ${group.id}`)
      for (const item of group.items) {
        assert.ok(t.nav[item], `${lang} is missing a nav label for ${item}`)
        assert.ok(t.desc[item], `${lang} is missing a description for ${item}`)
      }
    }
  }
})

test('settings-first ordering puts preferences before tools', () => {
  assert.equal(SETTINGS_GROUPS[0].id, 'general')
  assert.equal(settingsNavGroupOf('logs'), 'system')
  assert.equal(settingsNavGroupOf('models'), 'agent')
  assert.equal(settingsNavGroupOf('nope'), '')
})

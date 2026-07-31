import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const css = readFileSync(resolve(here, '../style.css'), 'utf8')

const ruleBodies = (selector) => {
  const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
  const matches = [...css.matchAll(new RegExp(`${escaped}\\s*\\{([^}]*)\\}`, 'g'))]
  assert.ok(matches.length > 0, `missing CSS rule for ${selector}`)
  return matches.map(match => match[1])
}

test('log-view keeps a readable foreground over its forced dark background', () => {
  const sharedPanelRules = ruleBodies('.log-panel pre, .preview pre, .artifact-view, .json-editor, .file-editor')
  assert.ok(
    sharedPanelRules.some(rule => /color\s*:\s*var\(--text\)\s*!important/i.test(rule)),
    'expected a shared panel rule that can force dark text with !important',
  )

  const logViewRule = ruleBodies('.log-panel pre.log-view')
    .find(rule => /background\s*:\s*#0f1115\s*!important/i.test(rule))
  assert.ok(logViewRule, 'missing forced dark log-view background rule')
  assert.match(logViewRule, /color\s*:\s*#d7e1ea\s*!important/i)
})

test('shared status feedback stays keyboard-visible and readable at narrow widths', () => {
  const focusRule = ruleBodies('.ga-status-actions button:focus-visible').join('\n')
  assert.match(focusRule, /outline\s*:\s*2px\s+solid/i)
  assert.match(focusRule, /outline-offset\s*:\s*2px/i)

  assert.match(
    css,
    /@media\s*\(max-width:\s*620px\)[\s\S]*?\.ga-status-notice\s*\{[^}]*max-width\s*:\s*100%[^}]*\}/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*620px\)[\s\S]*?\.ga-status-message\s*\{[^}]*white-space\s*:\s*normal[^}]*overflow-wrap\s*:\s*anywhere[^}]*\}/i,
  )
})

test('sidebar status notice fits its compact rail without hiding actions', () => {
  const noticeRule = ruleBodies('.ga-status-notice').join('\n')
  assert.match(noticeRule, /display\s*:\s*inline-grid/i)
  assert.match(noticeRule, /grid-template-columns\s*:\s*auto\s+minmax\(0,\s*1fr\)\s+auto/i)
  assert.match(noticeRule, /min-width\s*:\s*0/i)
  assert.match(noticeRule, /box-sizing\s*:\s*border-box/i)

  const sidebarRule = ruleBodies('.sidebar > .ga-status-notice').join('\n')
  assert.match(sidebarRule, /width\s*:\s*100%/i)
  assert.match(sidebarRule, /grid-template-columns\s*:\s*auto\s+minmax\(0,\s*1fr\)/i)

  const messageRule = ruleBodies('.sidebar > .ga-status-notice .ga-status-message').join('\n')
  assert.match(messageRule, /white-space\s*:\s*normal/i)
  assert.match(messageRule, /overflow-wrap\s*:\s*anywhere/i)

  const actionsRule = ruleBodies('.sidebar > .ga-status-notice .ga-status-actions').join('\n')
  assert.match(actionsRule, /width\s*:\s*100%/i)
})

test('language controls reserve stable space for translated labels', () => {
  assert.match(css, /\.sidebar \.lang-switch\s*\{[^}]*grid-template-columns:\s*minmax\(0, 1fr\) auto/s)
  assert.match(css, /\.sidebar \.lang-switch-label\s*\{[^}]*white-space:\s*nowrap/s)
  assert.match(
    css,
    /html\[data-color-scheme="dark"\] \.app:not\(\.app-tab-chat\) \.sidebar nav button\.active,[\s\S]*?\{[^}]*background:\s*var\(--surface-muted\)\s*!important[^}]*color:\s*var\(--text\)\s*!important/s,
  )
})

test('sent-message editor exposes keyboard focus and a narrow action layout', () => {
  const focusRule = ruleBodies('.oa-message-editor-actions button:focus-visible').join('\n')
  assert.match(focusRule, /outline\s*:\s*2px\s+solid/i)
  assert.match(focusRule, /outline-offset\s*:\s*2px/i)

  assert.match(
    css,
    /@media\s*\(max-width:\s*520px\)[^{]*\{[\s\S]*?\.oa-message-editor-hint\s*\{[^}]*white-space\s*:\s*normal[^}]*\}/i,
  )
})

test('model discovery keeps focus, responsive controls, and reduced-motion meaning', () => {
  const focusRule = ruleBodies('.model-discover-modal .model-candidate-item:focus-visible').join('\n')
  assert.match(focusRule, /outline\s*:\s*2px\s+solid/i)

  assert.match(
    css,
    /@media\s*\(max-width:\s*620px\)[\s\S]*?\.models-page \.model-discover-row\s*\{[^}]*flex-direction\s*:\s*column[^}]*\}/i,
  )
  assert.match(
    css,
    /@media\s*\(prefers-reduced-motion:\s*reduce\)[\s\S]*?\.model-discover-modal \.is-spinning\s*\{[^}]*animation\s*:\s*none[^}]*\}/i,
  )
  assert.match(
    css,
    /@media\s*\(prefers-reduced-motion:\s*reduce\)[\s\S]*?\.ga-status-pending \.ga-status-mark\s*\{[^}]*animation\s*:\s*none[^}]*\}/i,
  )
})

test('settings auto-title card has toggle and action field styles', () => {
  const toggleRule = ruleBodies('.settings-field-toggle').join('\n')
  assert.match(toggleRule, /grid-template-columns\s*:\s*minmax\(0,\s*1fr\)\s+auto/i)
  const stateRule = ruleBodies('.settings-toggle-state').join('\n')
  assert.match(stateRule, /border-radius\s*:\s*999px/i)
  assert.match(stateRule, /font-weight\s*:\s*700/i)
  const actionRule = ruleBodies('.settings-field-action').join('\n')
  assert.match(actionRule, /justify-items\s*:\s*end/i)
  assert.match(
    css,
    /\.settings-toggle-state\.is-on\s*\{[^}]*color\s*:\s*var\(--settings-success-ink\)[^}]*\}/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*760px\)[\s\S]*?\.settings-field-toggle\s*\{[^}]*grid-template-columns\s*:\s*minmax\(0,\s*1fr\)[^}]*\}/i,
  )
})


test('theme IDs only select token scopes while color schemes own shared compatibility', () => {
  assert.match(css, /html\[data-color-scheme="light"\]\s*\{\s*color-scheme:\s*light;\s*\}/i)
  assert.match(css, /html\[data-color-scheme="dark"\]\s*\{\s*color-scheme:\s*dark;\s*\}/i)

  const themeRules = [...css.matchAll(/([^{}]*\[data-theme="(?:light|warm|dark)"\][^{}]*)\{([^{}]*)\}/g)]
  assert.ok(themeRules.length >= 3, 'expected a token scope for every registered theme')

  for (const [, selector, body] of themeRules) {
    const declarations = body
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .split(';')
      .map(value => value.trim())
      .filter(Boolean)
    assert.ok(declarations.length > 0, `empty theme token scope: ${selector.trim()}`)
    assert.ok(
      declarations.every(declaration => declaration.startsWith('--')),
      `theme scope contains a structural declaration: ${selector.trim()}`,
    )
  }

  assert.equal((css.match(/\[data-theme="dark"\]/g) || []).length, 1)
  assert.ok((css.match(/\[data-color-scheme="dark"\]/g) || []).length > 1)
})

test('chat layout is shared instead of being coupled to the light palette', () => {
  const lightChatRule = ruleBodies('html[data-theme="light"] .oa-chat').join('\n')
  assert.doesNotMatch(lightChatRule, /(?:height|display|grid-template-columns|overflow|transition)\s*:/i)

  const sharedChatRule = ruleBodies('.oa-chat')
    .find(rule => /grid-template-columns\s*:\s*260px\s+minmax\(0,\s*1fr\)/i.test(rule))
  assert.ok(sharedChatRule, 'missing shared 260px chat layout')
  assert.match(sharedChatRule, /height\s*:\s*100vh/i)
  assert.match(sharedChatRule, /overflow\s*:\s*hidden/i)
})

test('theme-specific metadata and settings actions keep readable foregrounds', () => {
  const warmChatRule = ruleBodies('.oa-chat').join('\n')
  const lightChatRule = ruleBodies('html[data-theme="light"] .oa-chat').join('\n')
  const darkChatRule = ruleBodies('html[data-color-scheme="dark"] .oa-chat').join('\n')
  assert.match(warmChatRule, /--oa-faint\s*:\s*#756f66/i)
  assert.match(lightChatRule, /--oa-faint\s*:\s*#737373/i)
  assert.match(darkChatRule, /--oa-faint\s*:\s*#9299a4/i)
  assert.doesNotMatch(css, /--oa-faint\s*:\s*#[0-9a-f]{7}(?![0-9a-f])/i)

  const usageInlineRule = ruleBodies('.oa-usage.oa-usage-inline').join('\n')
  const usageTotalRule = ruleBodies('.oa-usage.oa-usage-total').join('\n')
  assert.match(usageInlineRule, /opacity\s*:\s*1/i)
  assert.match(usageTotalRule, /opacity\s*:\s*1/i)
  assert.match(css, /\.oa-turn-toggle:hover \.oa-usage-inline,\s*\.oa-turn-current-head:hover \.oa-usage-inline\s*\{[^}]*opacity\s*:\s*1/i)
  assert.match(ruleBodies('.oa-usage.oa-usage-total:hover').join('\n'), /opacity\s*:\s*1/i)
  assert.match(ruleBodies('.oa-message.user .oa-msg-meta').join('\n'), /color\s*:\s*var\(--oa-faint\)/i)

  const settingsRules = ruleBodies('.settings-page').join('\n')
  const darkSettingsRules = ruleBodies('html[data-color-scheme="dark"] .settings-page').join('\n')
  assert.match(settingsRules, /--settings-success-ink\s*:\s*#176b3c/i)
  assert.match(darkSettingsRules, /--settings-success-ink\s*:\s*#84e1c0/i)
  assert.match(ruleBodies('.settings-config-status.ready').join('\n'), /color\s*:\s*var\(--settings-success-ink\)/i)
  assert.match(ruleBodies('.settings-toggle-state.is-on').join('\n'), /color\s*:\s*var\(--settings-success-ink\)/i)

  const darkSettingsPrimary = ruleBodies('html[data-color-scheme="dark"] .settings-page button.primary').join('\n')
  assert.match(darkSettingsPrimary, /color\s*:\s*#062e25/i)
})

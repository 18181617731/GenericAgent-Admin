import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const here = dirname(fileURLToPath(import.meta.url))
const css = readFileSync(resolve(here, '../style.css'), 'utf8').replace(/\r\n?/g, '\n')

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
  const segmented = ruleBodies('.set-segmented button').join('\n')
  assert.match(segmented, /min-width\s*:\s*74px/i)
  assert.match(segmented, /white-space\s*:\s*nowrap/i)
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

test('settings rows render a real switch, a state pill, and stack on narrow screens', () => {
  assert.match(ruleBodies('.set-toggle-track').join('\n'), /border-radius\s*:\s*999px/i)
  assert.match(css, /\.set-toggle input:checked \+ \.set-toggle-track \.set-toggle-knob\s*\{[^}]*transform\s*:\s*translateX/i)
  const stateRule = ruleBodies('.settings-toggle-state').join('\n')
  assert.match(stateRule, /border-radius\s*:\s*999px/i)
  assert.match(stateRule, /font-weight\s*:\s*700/i)
  assert.match(ruleBodies('.set-card-footer').join('\n'), /justify-content\s*:\s*flex-end/i)
  assert.match(
    css,
    /\.settings-toggle-state\.is-on\s*\{[^}]*color\s*:\s*var\(--settings-success-ink\)[^}]*\}/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*720px\)[\s\S]*?\.set-row\s*\{[^}]*flex-direction\s*:\s*column[^}]*\}/i,
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
    .find(rule => /grid-template-columns\s*:\s*320px\s+minmax\(0,\s*1fr\)/i.test(rule))
  assert.ok(sharedChatRule, 'missing shared 320px chat layout')
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
  assert.match(ruleBodies('.set-state.is-on').join('\n'), /color\s*:\s*var\(--settings-success-ink/i)
  assert.match(ruleBodies('.settings-toggle-state.is-on').join('\n'), /color\s*:\s*var\(--settings-success-ink\)/i)

  const darkSettingsPrimary = ruleBodies('html[data-color-scheme="dark"] .settings-page button.primary').join('\n')
  assert.match(darkSettingsPrimary, /color\s*:\s*#062e25/i)
})

test('mobile turn headers hide token metadata while keeping the current status readable', () => {
  assert.match(
    css,
    /@media\s*\(max-width:\s*900px\)[\s\S]*?\.oa-turn-toggle \.oa-usage-inline,\s*\.oa-turn-current-head \.oa-usage-inline\s*\{\s*display\s*:\s*none\s*;\s*\}/i,
  )
  assert.match(
    css,
    /\.oa-turn-current-head \.oa-usage-inline \+ em\s*\{\s*margin-left\s*:\s*auto\s*;\s*\}/i,
  )
  const inlineBaseIndex = css.indexOf('.oa-usage.oa-usage-inline {')
  const mobileHideIndex = css.indexOf('.oa-turn-toggle .oa-usage-inline,\n  .oa-turn-current-head .oa-usage-inline { display:none; }')
  assert.ok(inlineBaseIndex >= 0, 'missing inline usage base rule')
  assert.ok(mobileHideIndex > inlineBaseIndex, 'mobile hide rule must follow inline usage base styles')
})

test('chat markdown tables use explicit warm, light, and dark palette tokens', () => {
  for (const selector of ['html[data-theme="warm"]', 'html[data-theme="light"]', 'html[data-theme="dark"]']) {
    const themeRules = ruleBodies(selector).filter(rule => /--chat-table-border\s*:/i.test(rule))
    assert.ok(themeRules.length > 0, `missing chat table tokens for ${selector}`)
    assert.match(themeRules.join('\n'), /--chat-table-header-bg\s*:/i)
  }

  const wrapRule = ruleBodies('.oa-table-wrap').join('\n')
  const cellRule = ruleBodies('.oa-md-table th,.oa-md-table td').join('\n')
  const headerRule = ruleBodies('.oa-md-table th').join('\n')
  assert.match(wrapRule, /border\s*:\s*1px\s+solid\s+var\(--chat-table-border\)/i)
  assert.match(cellRule, /border-right\s*:\s*1px\s+solid\s+var\(--chat-table-border\)/i)
  assert.match(cellRule, /border-bottom\s*:\s*1px\s+solid\s+var\(--chat-table-border\)/i)
  assert.match(headerRule, /background\s*:\s*var\(--chat-table-header-bg\)/i)
})

test('warm chat metadata clears AA contrast on translucent panels', () => {
  const declaration = (body, name) => {
    const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
    const match = body.match(new RegExp(`${escaped}\\s*:\\s*([^;]+)`, 'i'))
    assert.ok(match, `missing ${name} declaration`)
    return match[1].trim()
  }
  const color = (value) => {
    const hex = value.match(/^#([0-9a-f]{6})$/i)
    if (hex) {
      const packed = Number.parseInt(hex[1], 16)
      return { rgb: [(packed >> 16) & 255, (packed >> 8) & 255, packed & 255], alpha: 1 }
    }
    const rgba = value.match(/^rgba\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*([\d.]+)\s*\)$/i)
    assert.ok(rgba, `unsupported CSS color: ${value}`)
    return { rgb: rgba.slice(1, 4).map(Number), alpha: Number(rgba[4]) }
  }
  const over = (foreground, background) => ({
    rgb: foreground.rgb.map((channel, index) => (
      channel * foreground.alpha + background.rgb[index] * (1 - foreground.alpha)
    )),
    alpha: 1,
  })
  const luminance = ({ rgb }) => rgb
    .map(channel => channel / 255)
    .map(channel => (channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4))
    .reduce((sum, channel, index) => sum + channel * [0.2126, 0.7152, 0.0722][index], 0)
  const contrast = (first, second) => {
    const values = [luminance(first), luminance(second)].sort((a, b) => b - a)
    return (values[0] + 0.05) / (values[1] + 0.05)
  }

  const warmThemeRule = ruleBodies('html[data-theme="warm"]')
    .find(rule => /--surface-strong\s*:/i.test(rule))
  const warmChatRule = ruleBodies('.oa-chat')
    .find(rule => /--oa-muted\s*:/i.test(rule))
  const rootPaletteRule = ruleBodies(':root')
    .find(rule => /--g-ffffffa78\s*:/i.test(rule))
  assert.ok(warmThemeRule, 'missing warm theme surface tokens')
  assert.ok(warmChatRule, 'missing warm chat color tokens')
  assert.ok(rootPaletteRule, 'missing shared light alpha palette')

  const resolveColor = (value) => {
    const reference = value.match(/^var\((--[\w-]+)\)$/i)
    return color(reference ? declaration(rootPaletteRule, reference[1]) : value)
  }
  const worldlineKindRule = ruleBodies('.oa-worldline-kind').join('\n')
  assert.match(worldlineKindRule, /background\s*:\s*var\(--oa-hover\)/i)
  assert.match(worldlineKindRule, /color\s*:\s*var\(--oa-muted\)/i)

  const page = resolveColor(declaration(warmThemeRule, '--surface-strong'))
  const drawer = over(resolveColor(declaration(warmChatRule, '--oa-panel')), page)
  const badge = over(resolveColor(declaration(warmChatRule, '--oa-hover')), drawer)
  const muted = resolveColor(declaration(warmChatRule, '--oa-muted'))
  const ratio = contrast(muted, badge)

  assert.ok(ratio >= 4.5, `warm muted contrast ${ratio.toFixed(2)} is below WCAG AA`)
})

test('chat topbar tools share one theme-aware segmented control', () => {
  const group = ruleBodies('.oa-topbar-tools').join('\n')
  const base = ruleBodies('.oa-topbar-tools > .oa-context-btn').join('\n')
  const badge = ruleBodies('.oa-topbar-tools .oa-context-btn span').join('\n')
  const hover = ruleBodies('.oa-topbar-tools .oa-context-btn:hover:not(:disabled)').join('\n')
  const open = ruleBodies('.oa-topbar-tools .oa-context-btn.is-open').join('\n')
  const openBadge = ruleBodies('.oa-topbar-tools .oa-context-btn.is-open span').join('\n')
  const focus = ruleBodies('.oa-topbar-tools .oa-context-btn:focus-visible').join('\n')
  const disabled = ruleBodies('.oa-topbar-tools .oa-context-btn:disabled').join('\n')

  assert.match(group, /background\s*:\s*color-mix\([^;]*var\(--oa-panel\)/i)
  assert.match(group, /border\s*:\s*1px\s+solid\s+var\(--oa-line-strong\)/i)
  assert.match(base, /background\s*:\s*transparent/i)
  assert.match(base, /border-left\s*:\s*1px\s+solid\s+var\(--oa-line\)/i)
  assert.match(base, /color\s*:\s*var\(--oa-text\)/i)
  assert.match(badge, /background\s*:\s*var\(--oa-hover\)/i)
  assert.match(badge, /color\s*:\s*var\(--oa-muted\)/i)
  assert.match(hover, /background\s*:\s*var\(--oa-hover\)/i)
  assert.match(open, /background\s*:\s*var\(--oa-hover\)/i)
  assert.match(open, /color\s*:\s*var\(--oa-text\)/i)
  assert.match(open, /box-shadow\s*:\s*inset\s+0\s+-2px\s+var\(--oa-green\)\s*!important/i)
  assert.match(openBadge, /background\s*:\s*var\(--oa-panel\)/i)
  assert.match(openBadge, /color\s*:\s*var\(--oa-text\)/i)
  assert.match(focus, /box-shadow\s*:\s*inset\s+0\s+0\s+0\s+2px\s+var\(--oa-green\)\s*!important/i)
  assert.match(disabled, /opacity\s*:/i)

  const componentRules = [group, base, badge, hover, open, openBadge, focus, disabled].join('\n')
  assert.doesNotMatch(componentRules, /(?:#(?:000|111|fff)(?:fff)?\b|var\(--(?:d-|n-ffffff|i-111111|i-222222))/i)
  assert.doesNotMatch(css, /html\[data-color-scheme="dark"\][^{]*\.oa-context-btn/i)
})

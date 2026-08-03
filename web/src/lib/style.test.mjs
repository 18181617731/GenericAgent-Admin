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

test('product usability styles preserve keyboard focus, touch targets, and reduced motion', () => {
  assert.match(css, /:focus-visible\s*\{[^}]*outline\s*:\s*3px/is)
  assert.match(css, /@media\s*\(max-width:\s*680px\)[\s\S]*min-height\s*:\s*44px/i)
  assert.match(css, /@media\s*\(prefers-reduced-motion:\s*reduce\)[\s\S]*animation-duration\s*:\s*\.001ms/i)
  assert.match(css, /html\[data-theme="dark"\]\s+\.ga-message-banner\.is-error/i)
})

test('git sync logs stay bounded and scrollable', () => {
  const logRule = ruleBodies('.mini-log')[0]
  assert.match(logRule, /max-height\s*:\s*280px/i)
  assert.match(logRule, /overflow\s*:\s*auto/i)
  assert.match(logRule, /white-space\s*:\s*pre-wrap/i)
  assert.match(css, /@media\s*\(max-width:\s*680px\)[\s\S]*\.mini-log\s*\{[^}]*max-height\s*:\s*220px/i)
})

test('autonomous execution records stay uniformly left aligned', () => {
  const reportButtonRule = ruleBodies('.autonomous-report-list button').join('\n')
  assert.match(reportButtonRule, /grid-template-columns\s*:\s*minmax\(0,\s*1fr\)/i)
  assert.match(reportButtonRule, /justify-content\s*:\s*stretch/i)
  assert.match(reportButtonRule, /justify-items\s*:\s*start/i)
  assert.match(reportButtonRule, /text-align\s*:\s*left\s*!important/i)
})

test('mobile chat keeps semantic colors for total usage metrics', () => {
  for (const [selector, color] of [
    ['span.oa-usage-time', '#7c3aed'],
    ['span.oa-usage-in', '#2563eb'],
    ['span.oa-usage-cache', '#b66b00'],
    ['span.oa-usage-out', '#08785f'],
  ]) {
    assert.match(css, new RegExp(`@media\\s*\\(max-width:\\s*680px\\)[\\s\\S]*?\\.oa-usage\\.oa-usage-total\\s+${selector.replaceAll('.', '\\.')}\\s*\\{[^}]*color\\s*:\\s*${color}`, 'i'))
  }
})

test('touch sidebars expose actions without an iOS hover-first tap', () => {
  assert.match(css, /@media\s*\(hover:\s*none\),\s*\(pointer:\s*coarse\)[\s\S]*?\.oa-session-more\s*\{[^}]*opacity\s*:\s*1[^}]*pointer-events\s*:\s*auto/i)
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

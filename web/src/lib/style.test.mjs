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

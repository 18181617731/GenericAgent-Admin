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

test('autonomous execution records stay uniformly left aligned', () => {
  const reportButtonRule = ruleBodies('.autonomous-report-list button').join('\n')
  assert.match(reportButtonRule, /grid-template-columns\s*:\s*minmax\(0,\s*1fr\)/i)
  assert.match(reportButtonRule, /justify-content\s*:\s*stretch/i)
  assert.match(reportButtonRule, /justify-items\s*:\s*start/i)
  assert.match(reportButtonRule, /text-align\s*:\s*left\s*!important/i)
})

test('autonomous approvals expose readable outcomes and mobile bulk actions', () => {
  const toolbarRule = ruleBodies('.autonomous-approval-toolbar').join('\n')
  assert.match(toolbarRule, /flex-wrap\s*:\s*wrap/i)
  assert.match(css, /\.autonomous-approval-outcome\s*\{[^}]*border-left\s*:\s*3px/i)
  assert.match(css, /@media\s*\(max-width:\s*680px\)[\s\S]*?\.autonomous-approval-bulk-actions\s*\{[^}]*display\s*:\s*grid/i)
})

test('mobile chat keeps semantic colors for total usage metrics', () => {
  for (const [selector, color] of [
    ['span.oa-usage-time', '#7c3aed'],
    ['span.oa-usage-in', '#2563eb'],
    ['span.oa-usage-cache', '#b66b00'],
    ['span.oa-usage-out', '#08785f'],
  ]) {
    assert.match(css, new RegExp(`@media\\s*\\(max-width:\\s*680px\\)[\\s\\S]*?\\.oa-usage\\.oa-usage-total\\s+${selector.replaceAll('.', '\\.') }\\s*\\{[^}]*color\\s*:\\s*${color}`, 'i'))
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
    /\.settings-toggle-state\.is-on\s*\{[^}]*color\s*:\s*var\(--green\)[^}]*\}/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*760px\)[\s\S]*?\.settings-field-toggle\s*\{[^}]*grid-template-columns\s*:\s*minmax\(0,\s*1fr\)[^}]*\}/i,
  )
})

test('file preview expands into the remaining page height', () => {
  const previewContentRule = ruleBodies('.files-preview-content').join('\n')
  assert.match(previewContentRule, /display\s*:\s*flex/i)
  assert.match(previewContentRule, /flex\s*:\s*1\s+1\s+auto/i)
  assert.match(css, /@media\s*\(min-width:\s*681px\)[\s\S]*?\.files-preview-panel\s*\{[^}]*height\s*:\s*calc\(100dvh\s*-\s*64px\)/i)
  assert.match(css, /\.files-preview-content\s*> \.file-markdown-preview,[\s\S]*?max-height\s*:\s*none/i)
})

test('overview metrics expose visible and keyboard-friendly navigation affordances', () => {
  const linkRule = ruleBodies('.overview-page .overview-stats .stat-link').join('\n')
  assert.match(linkRule, /cursor\s*:\s*pointer/i)
  assert.match(css, /\.overview-page \.overview-stats\s*\{[^}]*grid-template-columns\s*:\s*repeat\(2,/i)
  assert.match(css, /\.overview-page \.overview-stats \.stat-link-icon\s*\{[^}]*position\s*:\s*absolute/i)
  assert.match(css, /\.app:not\(\.app-tab-chat\) \.app-page-header\s*\{\s*display\s*:\s*none/i)
})

test('approval review status separates unavailable models from rule-only screening', () => {
  const reviewRule = ruleBodies('.autonomous-approval-review').join('\n')
  assert.match(reviewRule, /display\s*:\s*grid/i)
  assert.match(css, /\.autonomous-approval-review\.is-unavailable[\s\S]*border-left-color\s*:\s*#dc2626/i)
  assert.match(css, /\.autonomous-approval-review\.is-rules[\s\S]*border-left-color\s*:\s*#d97706/i)
  assert.match(css, /\.autonomous-approval-review-meta\s*\{[^}]*display\s*:\s*flex/i)
  assert.match(css, /\.autonomous-approval-reason\s*\{[^}]*border-left\s*:\s*3px/i)
})

test('chat context and worldline buttons inherit every theme palette', () => {
  const base = ruleBodies('.oa-context-btn').join('\n')
  const badge = ruleBodies('.oa-context-btn span').join('\n')
  const hover = ruleBodies('.oa-context-btn:hover:not(:disabled)').join('\n')
  const open = ruleBodies('.oa-context-btn.is-open').join('\n')
  const openBadge = ruleBodies('.oa-context-btn.is-open span').join('\n')
  const disabled = ruleBodies('.oa-context-btn:disabled').join('\n')

  assert.match(base, /background\s*:\s*var\(--oa-panel\)/i)
  assert.match(base, /border\s*:\s*1px\s+solid\s+var\(--oa-line-strong\)/i)
  assert.match(base, /color\s*:\s*var\(--oa-text\)/i)
  assert.match(badge, /background\s*:\s*var\(--oa-hover\)/i)
  assert.match(badge, /color\s*:\s*var\(--oa-muted\)/i)
  assert.match(hover, /background\s*:\s*var\(--oa-hover\)/i)
  assert.match(open, /background\s*:\s*var\(--oa-text\)/i)
  assert.match(open, /color\s*:\s*var\(--oa-panel\)/i)
  assert.match(openBadge, /background\s*:\s*var\(--oa-panel\)/i)
  assert.match(openBadge, /color\s*:\s*var\(--oa-text\)/i)
  assert.match(disabled, /opacity\s*:/i)

  const componentRules = [base, badge, hover, open, openBadge, disabled].join('\n')
  assert.doesNotMatch(componentRules, /(?:#(?:000|111|fff)(?:fff)?\b|var\(--(?:d-|n-ffffff|i-111111|i-222222))/i)
  assert.doesNotMatch(css, /html\[data-color-scheme="dark"\][^{]*\.oa-context-btn/i)
})

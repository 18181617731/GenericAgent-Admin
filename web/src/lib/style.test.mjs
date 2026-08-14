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

test('interface scale controls are visible and scale the whole workspace', () => {
  assert.match(css, /html\s*\{[^}]*--ga-ui-scale\s*:\s*1/i)
  assert.match(css, /@supports\s*\(zoom\s*:\s*1\)[\s\S]*html\s*\{[^}]*zoom\s*:\s*var\(--ga-ui-scale/i)
  assert.match(css, /\.ui-scale-picker\s*\{[^}]*display\s*:\s*flex/i)
  assert.match(css, /\.ui-scale-button[^}]*min-height\s*:\s*32px/i)
  assert.match(css, /\.ui-scale-picker--settings\s*\{[^}]*grid-column\s*:\s*1\s*\/\s*-1/i)
  assert.match(css, /@media\s*\(max-width:\s*680px\)[\s\S]*?\.sidebar\s+\.ui-scale-picker\s*\{[^}]*display\s*:\s*none/i)
  assert.match(css, /\.oa-mobile-tools-menu\s*\{[\s\S]*?max-width\s*:\s*calc\(100%\s*-\s*16px\)/i)
  assert.match(css, /\.ui-scale-picker--mobile\s*\{[^}]*background\s*:/i)
  assert.match(
    css,
    /@supports\s*\(zoom\s*:\s*1\)[\s\S]*?\.oa-chat,[\s\S]*?height\s*:\s*calc\(100dvh\s*\*\s*var\(--ga-ui-scale-width/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*900px\)[\s\S]*?\.oa-chat\s+\.oa-sidebar\s*\{[^}]*width\s*:\s*min\([\s\S]*?var\(--ga-ui-scale-width/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*900px\)[\s\S]*?\.oa-chat,[\s\S]*?\.oa-chat\.is-collapsed\s*\{[^}]*height\s*:\s*calc\(100dvh\s*\*\s*var\(--ga-ui-scale-width/i,
  )
  assert.match(
    css,
    /@media\s*\(min-width:\s*901px\)[\s\S]*?\.oa-chat,[\s\S]*?\.oa-chat\s+\.oa-main,[\s\S]*?\.oa-chat\s+\.oa-sidebar\s*\{[^}]*height\s*:\s*calc\(100dvh\s*\*\s*var\(--ga-ui-scale-width[^}]*!important/i,
  )
  assert.match(css, /\.oa-chat \.oa-sidebar-backdrop\s*\{[^}]*width:\s*calc\(100vw \* var\(--ga-ui-scale-width, 1\)\)[^}]*height:\s*calc\(100dvh \* var\(--ga-ui-scale-width, 1\)\)/s)
  assert.match(css, /\.oa-chat \.oa-session-more\s*\{[^}]*min-height:\s*calc\(44px \* var\(--ga-ui-scale-width, 1\)\)/s)
  assert.match(css, /\.oa-chat \.oa-session-menu button\s*\{[^}]*min-height:\s*calc\(48px \* var\(--ga-ui-scale-width, 1\)\)/s)
  assert.match(css, /\.oa-session-search-backdrop,[\s\S]*?\.oa-session-manager-backdrop\s*\{[^}]*width:\s*calc\(100vw \* var\(--ga-ui-scale-width, 1\)\)[^}]*height:\s*calc\(100dvh \* var\(--ga-ui-scale-width, 1\)\)/s)
})

test('admin sidebar appearance controls use separate rows at desktop width', () => {
  assert.match(
    css,
    /\.app:not\(\.app-tab-chat\)\s+#admin-sidebar\s+\.lang-switch\s*\{[^}]*display\s*:\s*grid[^}]*grid-template-columns\s*:\s*minmax\(0,\s*1fr\)\s+auto/i,
  )
  assert.match(
    css,
    /#admin-sidebar\s+\.lang-switch\s*>\s*\.theme-picker,[\s\S]*?#admin-sidebar\s+\.lang-switch\s*>\s*\.ui-scale-picker\s*\{[^}]*grid-column\s*:\s*1\s*\/\s*-1/i,
  )
  assert.match(
    css,
    /#admin-sidebar\s+\.ui-scale-picker\s*\{[^}]*display\s*:\s*grid[^}]*grid-template-columns\s*:\s*minmax\(0,\s*1fr\)\s+auto/i,
  )
  assert.match(css, /#admin-sidebar\s+\.ui-scale-label\s*>\s*span\s*\{[^}]*text-overflow\s*:\s*ellipsis/i)
  assert.match(
    css,
    /html\[data-ui-scale="80"\][\s\S]*?html\[data-ui-scale="90"\][\s\S]*?#admin-sidebar\s+\.ui-scale-picker\s*\{[^}]*grid-template-columns\s*:\s*minmax\(0,\s*1fr\)/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*860px\)[\s\S]*?#admin-sidebar\s+\.lang-switch\s*>\s*\.ui-scale-picker\s*\{[^}]*display\s*:\s*none/i,
  )
})

test('mobile feedback stays in document flow and model actions use two columns', () => {
  assert.match(
    css,
    /@media\s*\(max-width:\s*680px\)[\s\S]*?\.global-feedback\s*\{[^}]*position\s*:\s*relative[^}]*width\s*:\s*auto[^}]*padding\s*:\s*8px\s+8px\s+8px\s+13px/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*620px\)[\s\S]*?\.models-page\s+\.model-config-toolbar\s*\{[^}]*flex-direction\s*:\s*column/i,
  )
  assert.match(
    css,
    /\.models-page\s+\.model-config-toolbar\s*>\s*\.ant-space\s*\{[^}]*grid-template-columns\s*:\s*repeat\(2,/i,
  )
})

test('mobile chat navigation and tools stay fixed inside the iPhone viewport', () => {
  assert.match(
    css,
    /@media\s*\(max-width:\s*680px\)[\s\S]*?\.oa-chat\s+\.oa-topbar\s*\{[^}]*grid-template-columns\s*:\s*max-content\s+minmax\(0,\s*1fr\)/i,
  )
  assert.match(css, /@media\s*\(max-width:\s*680px\)[\s\S]*?\.oa-chat\s+\.oa-topbar\s*\{[^}]*grid-template-rows\s*:\s*48px\s+44px/i)
  assert.match(css, /\.oa-chat\s+\.oa-admin-back-trigger\s*\{[^}]*display\s*:\s*none/i)
  assert.match(css, /@media\s*\(max-width:\s*680px\)[\s\S]*?\.oa-chat\s+\.oa-collapsed-actions\s+\.oa-admin-back-trigger\s*\{[^}]*display\s*:\s*inline-flex/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-topbar\s*\{[^}]*position\s*:\s*fixed/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-topbar\s*\{[^}]*inset\s*:\s*0\s+0\s+auto/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-topbar\s*\{[^}]*width\s*:\s*100%/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-topbar-actions\s*\{[^}]*grid-row\s*:\s*2/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-workspace\s*\{[^}]*grid-row\s*:\s*2/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-workspace\s*>\s*\.oa-thread\s*\{[^}]*grid-row\s*:\s*auto/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-composer-wrap\s*\{[^}]*grid-row\s*:\s*3/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-chat\s+\.oa-topbar-actions\s*>\s*\.oa-context-btn\s*\{[^}]*display\s*:\s*inline-flex\s*!important/i)
  assert.match(css, /\.oa-mobile-tools-layer\s*\{[^}]*position\s*:\s*fixed[^}]*z-index\s*:\s*1000/i)
  assert.match(css, /\.oa-mobile-picker-backdrop\s*\{[^}]*box-sizing\s*:\s*border-box[^}]*padding-bottom\s*:\s*max\(4px,\s*env\(safe-area-inset-bottom\)\)/i)
  assert.match(
    css,
    /@media\s*\(max-width:680px\)[\s\S]*?\.notification-filter-scroll\s*\{[^}]*display\s*:\s*grid[^}]*grid-template-columns\s*:\s*repeat\(2,/i,
  )
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-topbar\s*\{[^}]*overflow-x\s*:\s*visible\s*!important[^}]*overflow-y\s*:\s*visible\s*!important/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.notification-popover\s*\{[^}]*left\s*:\s*0[^}]*right\s*:\s*0[^}]*width\s*:\s*auto/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.notification-popover\s*\{[^}]*z-index\s*:\s*1200/i)
})

test('desktop chat keeps the composer under the transcript and gives sidebars breathing room', () => {
  assert.match(css, /\.oa-main\.has-loop\s*\{[^}]*--oa-chat-rail-width\s*:\s*320px/i)
  assert.match(css, /\.oa-main\s*>\s*\.oa-composer-wrap\s*\{[^}]*justify-self\s*:\s*start/i)
  assert.match(css, /\.oa-main\s*>\s*\.oa-composer-wrap\s*\{[^}]*width\s*:\s*calc\(100%\s*-\s*var\(--oa-chat-rail-width\)/i)
  assert.match(css, /\.oa-main\s*>\s*\.oa-composer-wrap\s*\{[^}]*margin-right\s*:\s*var\(--oa-chat-rail-width/i)
  assert.match(css, /\.oa-main\s*>\s*\.oa-composer-wrap\s*\{[^}]*padding-left\s*:\s*max\(20px/i)
  assert.match(css, /\.oa-chat\s+\.oa-sidebar\s*\{[^}]*padding\s*:\s*14px\s+14px\s+12px/i)
  assert.match(css, /\.oa-chat\s+\.oa-session-list\s*\{[^}]*padding\s*:\s*4px\s+3px\s+12px/i)
  assert.match(css, /\.oa-workspace\.has-loop\s*\{[^}]*grid-template-columns\s*:\s*minmax\(0,\s*1fr\)\s+320px/i)
  assert.match(css, /\.oa-loop-panel\s*\{[^}]*display\s*:\s*grid/i)
  assert.match(css, /\.oa-loop-config\s*\{[^}]*display\s*:\s*grid/i)
})

test('loop configuration keeps model options above clipped rails and explains empty objectives', () => {
  assert.match(css, /\.oa-cselect-menu-portal\s*\{[^}]*position\s*:\s*fixed[^}]*z-index\s*:\s*1400/i)
  assert.match(css, /\.oa-loop-config-actions\s+small\.is-warning\s*\{[^}]*color\s*:\s*var\(--red\)/i)
  assert.match(css, /\.oa-loop-demo\s*\{[^}]*border\s*:/i)
  assert.match(css, /\.oa-loop-demo-steps\s*\{[^}]*display\s*:\s*grid/i)
  assert.match(css, /\.oa-loop-guide\s*\{[^}]*border\s*:/i)
})

test('mobile density pass keeps the shell compact without shrinking touch controls into text', () => {
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.app:not\(\.app-tab-chat\)\s+\.sidebar\s+nav\s*\{\s*display\s*:\s*none/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.sidebar,[\s\S]*?grid-template-columns\s*:\s*minmax\(0,\s*1fr\)\s+104px\s+36px/i)
  assert.match(css, /\.brand,[\s\S]*?\.app:not\(\.app-tab-chat\)\s+\.brand\s*\{[^}]*display:none/i)
  assert.match(css, /\.mobile-nav-trigger\s*\{[^}]*grid-column:1[^}]*grid-row:1/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.main,[\s\S]*?padding:8px\s+6px/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.stat\s*\{[^}]*min-height:64px/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.panel,[\s\S]*?padding:10px/i)
  assert.match(css, /@media\s*\(max-width:680px\)[\s\S]*?\.oa-main\s*\{[^}]*--oa-topbar-h:48px/i)
  assert.match(css, /\.sidebar\s+\.theme-picker-trigger-copy,[\s\S]*?\.theme-picker--compact\s+\.theme-picker-trigger\s*>\s*svg:last-child\s*\{\s*display:none/i)
  assert.match(css, /\.overview-page\s+\.overview-stats\s+\.stat\s*\{[^}]*min-height:64px[^}]*padding:8px\s+9px/i)
  assert.match(css, /\.overview-page\s*>\s*\.observability-card,[\s\S]*?padding:10px/i)
  assert.match(css, /@media\s*\(max-width:\s*420px\)[\s\S]*?\.overview-page\s+\.observability-stats\s*\{[^}]*grid-template-columns\s*:\s*1fr/i)
  assert.match(css, /\.schedule-stats\s*\{[^}]*grid-template-columns:repeat\(4,/i)
  assert.match(css, /\.channel-hero\s*>\s*div:first-child\s*\{\s*display:none/i)
  assert.match(css, /\.channel-toolbar\s+\.actions\s*\{[^}]*grid-template-columns:repeat\(2,/i)
  assert.match(css, /\.usage-metrics\s*\{[^}]*grid-template-columns:repeat\(2,/i)
  assert.match(css, /\.log-actions\s*\{[^}]*grid-template-columns:repeat\(2,/i)
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

test('autonomous approvals expose readable problem summaries and mobile bulk actions', () => {
  const toolbarRule = ruleBodies('.autonomous-approval-toolbar').join('\n')
  assert.match(toolbarRule, /flex-wrap\s*:\s*wrap/i)
  assert.match(css, /\.autonomous-approval-problem\s*\{[^}]*border-left\s*:\s*3px/i)
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

test('settings rows render a real switch, a state pill, and stack on narrow screens', () => {
  assert.match(ruleBodies('.set-toggle-track').join('\n'), /border-radius\s*:\s*999px/i)
  assert.match(css, /\.set-toggle input:checked \+ \.set-toggle-track \.set-toggle-knob\s*\{[^}]*transform\s*:\s*translateX/i)
  const stateRule = ruleBodies('.settings-toggle-state').join('\n')
  assert.match(stateRule, /border-radius\s*:\s*999px/i)
  assert.match(stateRule, /font-weight\s*:\s*700/i)
  assert.match(ruleBodies('.set-card-footer').join('\n'), /justify-content\s*:\s*flex-end/i)
  assert.match(
    css,
    /\.settings-toggle-state\.is-on\s*\{[^}]*color\s*:\s*var\(--green\)[^}]*\}/i,
  )
  assert.match(
    css,
    /@media\s*\(max-width:\s*720px\)[\s\S]*?\.set-row\s*\{[^}]*flex-direction\s*:\s*column[^}]*\}/i,
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

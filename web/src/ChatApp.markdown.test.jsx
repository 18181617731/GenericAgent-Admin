import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render } from '@testing-library/react'
import { ChatMessage } from './ChatApp.jsx'

afterEach(() => cleanup())

// Renders model output through the real assistant pipeline
// (ChatMessage -> AssistantContent -> MarkdownBlock -> TextMarkdown).
const renderAssistant = (content, messagePatch = {}) => render(
  <ChatMessage
    message={{ id: 'a-md', role: 'assistant', content, files: [], created_at: 0, ...messagePatch }}
    pending={false}
    onAskReply={vi.fn()}
  />,
).container

describe('assistant markdown rendering', () => {
  test('renders nested emphasis, code spans and links as real elements', () => {
    const container = renderAssistant('A **bold `snippet` and [link](https://a.test)** tail.')
    const strong = container.querySelector('.oa-md strong')
    expect(strong).toBeTruthy()
    expect(strong.querySelector('code').textContent).toBe('snippet')
    const link = strong.querySelector('a')
    expect(link.getAttribute('href')).toBe('https://a.test')
    expect(link.getAttribute('rel')).toContain('noopener')
  })

  test('renders underscore emphasis but leaves snake_case alone', () => {
    const container = renderAssistant('_stressed_ but not snake_case_name here')
    expect(container.querySelector('.oa-md em').textContent).toBe('stressed')
    expect(container.querySelector('.oa-md').textContent).toContain('snake_case_name')
  })

  test('renders file_patch calls as a dedicated file diff instead of raw JSON arguments', () => {
    const args = JSON.stringify({
      path: 'src/components/Demo.jsx',
      old_content: 'const value = 1\nconst stable = true',
      new_content: 'const value = 2\nconst stable = true',
    })
    const container = renderAssistant([
      '\u{1F6E0}\uFE0F Tool: `file_patch`',
      '```text',
      args,
      '```',
    ].join('\n'))

    const panel = container.querySelector('.oa-file-tool-args.is-patch')
    expect(panel).toBeTruthy()
    expect(panel.querySelector('.oa-patch-file-id strong').textContent).toBe('Demo.jsx')
    expect(panel.querySelector('.oa-patch-file-id span').textContent).toBe('src/components/Demo.jsx')
    expect(panel.querySelector('.oa-file-tool-badge')).toBeNull()
    expect(panel.querySelector('.oa-diff-stats-add').textContent).toBe('+1')
    expect(panel.querySelector('.oa-diff-stats-del').textContent).toBe('\u22121')
    const toggle = panel.querySelector('.oa-patch-toggle')
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(toggle.textContent).toContain('\u6536\u8d77')
    expect(panel.querySelector('.oa-diff-add .oa-diff-text').textContent).toBe('const value = 2')
    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(panel.querySelector('.oa-diff')).toBeNull()
    fireEvent.click(toggle)
    expect(panel.querySelector('.oa-diff')).toBeTruthy()
    expect(panel.querySelector('.oa-diff-del .oa-diff-text').textContent).toBe('const value = 1')
    expect(container.querySelector('.ga-tool-arg')).toBeNull()
  })

  test('starts a large file_patch collapsed and expands it on demand', () => {
    const before = Array.from({ length: 13 }, (_, i) => `const value${i} = ${i}`).join('\n')
    const after = Array.from({ length: 13 }, (_, i) => `const value${i} = ${i + 1}`).join('\n')
    const args = JSON.stringify({ path: 'src/large.js', old_content: before, new_content: after })
    const container = renderAssistant([
      '\u{1F6E0}\uFE0F Tool: `file_patch`',
      '```text',
      args,
      '```',
    ].join('\n'))

    const panel = container.querySelector('.oa-file-tool-args.is-patch')
    const toggle = panel.querySelector('.oa-patch-toggle')
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(toggle.textContent).toContain('\u5c55\u5f00')
    expect(panel.querySelector('.oa-diff')).toBeNull()
    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(panel.querySelectorAll('.oa-diff-add').length).toBe(13)
    expect(panel.querySelectorAll('.oa-diff-del').length).toBe(13)
  })

  test('renders file_write with the same compact file bar and collapsible diff as file_patch', () => {
    const args = JSON.stringify({
      path: 'src/generated/config.js',
      content: 'export const enabled = true\nexport const retries = 3',
      mode: 'append',
    })
    const container = renderAssistant([
      '\u{1F6E0}\uFE0F Tool: `file_write`',
      '```text',
      args,
      '```',
    ].join('\n'))

    const panel = container.querySelector('.oa-file-tool-args.is-patch.is-write')
    expect(panel).toBeTruthy()
    expect(panel.querySelector('.oa-patch-file-id strong').textContent).toBe('config.js')
    expect(panel.querySelector('.oa-patch-file-id > span').textContent).toBe('src/generated/config.js')
    expect(panel.querySelector('.oa-patch-mode').textContent).toBe('append')
    expect(panel.querySelector('.oa-file-tool-badge')).toBeNull()
    expect(panel.querySelector('.oa-file-tool-path')).toBeNull()
    expect(panel.querySelector('.oa-diff-stats-add').textContent).toBe('+2')
    expect(panel.querySelector('.oa-diff-stats-del').textContent).toBe('\u22120')
    const toggle = panel.querySelector('.oa-patch-toggle')
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(panel.querySelectorAll('.oa-diff-add').length).toBe(2)
    fireEvent.click(toggle)
    expect(panel.querySelector('.oa-diff')).toBeNull()
  })

  test('starts a large file_write collapsed and expands it on demand', () => {
    const content = Array.from({ length: 13 }, (_, i) => `line ${i + 1}`).join('\n')
    const args = JSON.stringify({ path: 'src/generated/large.txt', content, mode: 'overwrite' })
    const container = renderAssistant([
      '\u{1F6E0}\uFE0F Tool: `file_write`',
      '```text',
      args,
      '```',
    ].join('\n'))

    const panel = container.querySelector('.oa-file-tool-args.is-patch.is-write')
    const toggle = panel.querySelector('.oa-patch-toggle')
    expect(toggle.getAttribute('aria-expanded')).toBe('false')
    expect(panel.querySelector('.oa-diff')).toBeNull()
    expect(panel.querySelector('.oa-patch-mode')).toBeNull()
    fireEvent.click(toggle)
    expect(toggle.getAttribute('aria-expanded')).toBe('true')
    expect(panel.querySelectorAll('.oa-diff-add').length).toBe(13)
  })

  test('renders a blockquote as a quote element', () => {
    const container = renderAssistant('> quoted **note**\n> second line');
    const quote = container.querySelector('blockquote.oa-md-quote')
    expect(quote).toBeTruthy()
    expect(quote.querySelector('strong').textContent).toBe('note')
  })

  test('renders nested bullets as nested lists', () => {
    const container = renderAssistant('- outer\n  - inner\n- second')
    const outer = container.querySelector('.oa-md > ul.oa-list')
    expect(outer.children.length).toBe(2)
    const inner = outer.querySelector('ul.oa-list')
    expect(inner).toBeTruthy()
    expect(inner.textContent).toContain('inner')
  })

  test('renders task list items as checkboxes reflecting their state', () => {
    const container = renderAssistant('- [x] shipped\n- [ ] pending')
    const boxes = container.querySelectorAll('.oa-list-task input[type="checkbox"]')
    expect(boxes.length).toBe(2)
    expect(boxes[0].checked).toBe(true)
    expect(boxes[1].checked).toBe(false)
    expect(container.querySelector('.oa-task-item.is-done')).toBeTruthy()
  })

  test('renders an inline image instead of leaking a stray exclamation mark', () => {
    const container = renderAssistant('![diagram](https://a.test/d.png)')
    const image = container.querySelector('img.oa-md-image')
    expect(image.getAttribute('src')).toBe('https://a.test/d.png')
    expect(image.getAttribute('alt')).toBe('diagram')
    expect(container.querySelector('.oa-md').textContent).not.toContain('!')
  })

  test('refuses a javascript: destination and keeps the label readable', () => {
    const container = renderAssistant('[tap](javascript:alert(1))')
    expect(container.querySelector('.oa-md a')).toBeNull()
    expect(container.querySelector('.oa-md').textContent).toContain('tap')
  })

  test('renders a table whose delimiter row is short of the header', () => {
    const container = renderAssistant('| a | b | c |\n| --- | --- |\n| 1 | 2 | 3 |')
    const table = container.querySelector('table.oa-md-table')
    expect(table).toBeTruthy()
    expect(table.querySelectorAll('thead th').length).toBe(3)
    expect(table.querySelectorAll('tbody td').length).toBe(3)
  })

  test('keeps a list whole across a blank line and marks it loose', () => {
    const container = renderAssistant('- first\n\n- second')
    const lists = container.querySelectorAll('.oa-md ul.oa-list')
    expect(lists.length).toBe(1)
    expect(lists[0].children.length).toBe(2)
    expect(lists[0].classList.contains('oa-list-loose')).toBe(true)
  })

  test('autolinks a bare url without swallowing trailing punctuation', () => {
    const container = renderAssistant('docs at https://a.test/guide.')
    const link = container.querySelector('.oa-md a')
    expect(link.getAttribute('href')).toBe('https://a.test/guide')
    expect(container.querySelector('.oa-md').textContent).toContain('guide.')
  })

  test('still renders fenced code as a code card with its language', () => {
    const container = renderAssistant('before\n\n```go\nfmt.Println("x")\n```\n\nafter')
    const card = container.querySelector('.oa-code-card')
    expect(card).toBeTruthy()
    expect(card.querySelector('.oa-code-head span').textContent).toBe('go')
    expect(card.querySelector('pre code').textContent).toContain('fmt.Println')
  })

  test('renders bracketed display math through KaTeX', () => {
    const formula = '\\text{tok/s}=\\frac{\\sum \\text{\\u5df2\\u6d4b\\u91cf\\u6b65\\u9aa4\\u7684 output\\_tokens}}{\\sum \\text{generation\\_ms}/1000}'
    const container = renderAssistant(`\\[ ${formula} \\]`)
    const math = container.querySelector('.oa-math-display .katex-display')
    expect(math).toBeTruthy()
    expect(math.querySelector('annotation[encoding="application/x-tex"]').textContent).toBe(formula)
    expect(container.querySelector('.oa-md').textContent).not.toContain('\\[')
  })

  test('renders inline dollar math without parsing prices or code spans', () => {
    const container = renderAssistant('Energy $E=mc^2$, price $5 and $10, code `$x$`.')
    const math = container.querySelectorAll('.oa-math-inline .katex')
    expect(math.length).toBe(1)
    expect(math[0].querySelector('annotation[encoding="application/x-tex"]').textContent).toBe('E=mc^2')
    expect(container.querySelector('code').textContent).toBe('$x$')
    expect(container.querySelector('.oa-md').textContent).toContain('$5 and $10')
  })

  test('still uses structured blocks when the text has no multi-turn protocol', () => {
    const container = renderAssistant('fallback text', {
      structured_content: [
        { type: 'tool_use', id: 'toolu_1', name: 'file_read', input: { path: 'README.md' } },
        { type: 'text', text: 'structured answer' },
      ],
    })

    expect(container.querySelector('.oa-turn-stack')).toBeNull()
    expect(container.textContent).toContain('structured answer')
    expect(container.textContent).not.toContain('fallback text')
  })

  test('keeps the multi-turn UI when the terminal message adds structured content', () => {
    const content = [
      'LLM Running (Turn 1)',
      '<summary>inspect stream</summary>',
      'first body',
      '',
      'LLM Running (Turn 2)',
      '<summary>finish work</summary>',
      'second body',
      '',
      '```',
      '[Info] Final response to user.',
      '```',
      'final answer',
    ].join('\n')
    const structuredContent = [
      { type: 'thinking', thinking: 'terminal-only reasoning' },
      { type: 'text', text: 'final answer' },
    ]

    const container = renderAssistant(content, { structured_content: structuredContent })

    expect(container.querySelector('.oa-turn-stack')).toBeTruthy()
    expect(container.querySelector('.oa-turn-stack-head b').textContent).toBe('2')
    expect(container.querySelector('.oa-turn-current-head').textContent).toContain('finish work')
    expect(container.querySelector('.oa-final-answer').textContent).toContain('final answer')
    expect(container.textContent).not.toContain('terminal-only reasoning')
  })

  test('keeps a heading, rule and paragraph rhythm', () => {
    const container = renderAssistant('# Title\n\nbody text\n\n---\n\n## Next')
    const md = container.querySelector('.oa-md')
    expect(md.querySelector('h1').textContent).toBe('Title')
    expect(md.querySelector('h2').textContent).toBe('Next')
    expect(md.querySelector('hr')).toBeTruthy()
    expect(md.querySelector('p').textContent).toBe('body text')
  })
})

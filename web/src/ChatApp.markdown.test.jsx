import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, render } from '@testing-library/react'
import { ChatMessage } from './ChatApp.jsx'

afterEach(() => cleanup())

// Renders model output through the real assistant pipeline
// (ChatMessage -> AssistantContent -> MarkdownBlock -> TextMarkdown).
const renderAssistant = (content) => render(
  <ChatMessage
    message={{ id: 'a-md', role: 'assistant', content, files: [], created_at: 0 }}
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

  test('keeps a heading, rule and paragraph rhythm', () => {
    const container = renderAssistant('# Title\n\nbody text\n\n---\n\n## Next')
    const md = container.querySelector('.oa-md')
    expect(md.querySelector('h1').textContent).toBe('Title')
    expect(md.querySelector('h2').textContent).toBe('Next')
    expect(md.querySelector('hr')).toBeTruthy()
    expect(md.querySelector('p').textContent).toBe('body text')
  })
})

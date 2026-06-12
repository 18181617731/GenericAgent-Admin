import test from 'node:test'
import assert from 'node:assert/strict'
import { MARKDOWN_CHAR_LIMIT, MARKDOWN_LINE_LIMIT, parseAssistantContent, textRenderStats, previewLongText } from './chatTextSafety.js'

test('many content lines do not trigger safe preview by line count alone', () => {
  const text = Array.from({ length: MARKDOWN_LINE_LIMIT + 20 }, (_, i) => `line ${i}`).join('\n')
  const stats = textRenderStats(text)
  assert.equal(stats.lines, MARKDOWN_LINE_LIMIT + 20)
  assert.equal(stats.standaloneNewlineLines, 0)
  assert.equal(stats.tooLarge, false)
})

test('many standalone blank lines still trigger safe preview', () => {
  const text = Array.from({ length: MARKDOWN_LINE_LIMIT + 20 }, () => '').join('\n')
  const stats = textRenderStats(text)
  assert.equal(stats.lines, MARKDOWN_LINE_LIMIT + 20)
  assert.equal(stats.standaloneNewlineLines, MARKDOWN_LINE_LIMIT + 20)
  assert.equal(stats.tooLarge, true)
})

test('preview folds long runs of blank lines', () => {
  const preview = previewLongText(`a\n\n\n\n\n\n\n\n\n\nb`)
  assert.match(preview, /连续空行已折叠/)
})


test('assistant content is split into turns before large-text fallback', () => {
  const turnBody = 'x'.repeat(Math.floor(MARKDOWN_CHAR_LIMIT / 2))
  const text = [
    'LLM Running (Turn 1)',
    '<summary>first</summary>',
    turnBody,
    '',
    'LLM Running (Turn 2)',
    '<summary>second</summary>',
    turnBody,
    '',
    '```',
    '[Info] Final response to user.',
    '```',
    'final answer',
  ].join('\n')
  const fullStats = textRenderStats(text)
  assert.equal(fullStats.tooLarge, true)
  const parsed = parseAssistantContent(text)
  assert.equal(parsed.runs.length, 2)
  assert.equal(parsed.runs[0].title, 'first')
  assert.equal(textRenderStats(parsed.runs[0].body).tooLarge, false)
  assert.equal(textRenderStats(parsed.runs[1].body).tooLarge, false)
  assert.equal(parsed.body, 'final answer')
})

test('assistant parser ignores transcript markers inside fenced tool output', () => {
  const text = [
    'LLM Running (Turn 24)',
    '<summary>real 24</summary>',
    'before tool output',
    '```',
    'tool log includes a fake marker:',
    'LLM Running (Turn 25)',
    '<summary>fake turn in code fence</summary>',
    '```',
    'after tool output',
    '',
    'LLM Running (Turn 25)',
    '<summary>real 25</summary>',
    'real turn body',
    '',
    '```',
    '[Info] Final response to user.',
    '```',
    'final answer',
  ].join('\n')
  const parsed = parseAssistantContent(text)
  assert.equal(parsed.runs.length, 2)
  assert.equal(parsed.runs[0].turn, 24)
  assert.equal(parsed.runs[0].title, 'real 24')
  assert.match(parsed.runs[0].body, /fake marker/)
  assert.match(parsed.runs[0].body, /after tool output/)
  assert.equal(parsed.runs[1].turn, 25)
  assert.equal(parsed.runs[1].title, 'real 25')
  assert.equal(parsed.body, 'final answer')
})

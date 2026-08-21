import assert from 'node:assert/strict'
import test from 'node:test'
import { segmentAgentProtocolBlocks } from './agentProtocol.js'

const toolCall = (name, body) => [
  '\u{1F6E0}\uFE0F Tool: `' + name + '`',
  '```text',
  body,
  '```',
]

const resultBlock = (body) => ['`````', body, '`````']

const kinds = (segments) => segments.map((segment) => segment.kind)

test('keeps prose and tool folds in their original order', () => {
  const text = [
    'Before the tool',
    '',
    ...toolCall('demo', '{"ok":true}'),
    '',
    'After the tool',
  ].join('\n')

  const segments = segmentAgentProtocolBlocks(text)

  assert.deepEqual(kinds(segments), ['prose', 'folds', 'prose'])
  assert.equal(segments[0].text, 'Before the tool')
  assert.equal(segments[1].folds[0].label, 'demo')
  assert.equal(segments[1].folds[0].body, '{"ok":true}')
  assert.equal(segments[2].text, 'After the tool')
})

test('groups consecutive tools across blank lines', () => {
  const text = [
    'Before',
    '',
    ...toolCall('first', 'one'),
    '',
    '',
    ...toolCall('second', 'two'),
    '',
    'After',
  ].join('\n')

  const segments = segmentAgentProtocolBlocks(text)

  assert.deepEqual(kinds(segments), ['prose', 'folds', 'prose'])
  assert.deepEqual(segments[1].folds.map((fold) => fold.label), ['first', 'second'])
})

test('prose between tools starts a new fold group', () => {
  const text = [
    ...toolCall('first', 'one'),
    '',
    'Narration between tools',
    '',
    ...toolCall('second', 'two'),
  ].join('\n')

  const segments = segmentAgentProtocolBlocks(text)

  assert.deepEqual(kinds(segments), ['folds', 'prose', 'folds'])
  assert.equal(segments[0].folds.length, 1)
  assert.equal(segments[2].folds.length, 1)
})

test('attaches a following tool result to its tool fold', () => {
  const text = [
    ...toolCall('demo', 'arguments'),
    ...resultBlock('result body'),
    '',
    'Done',
  ].join('\n')

  const segments = segmentAgentProtocolBlocks(text)

  assert.deepEqual(kinds(segments), ['folds', 'prose'])
  assert.equal(segments[0].folds.length, 1)
  assert.equal(segments[0].folds[0].result, 'result body')
  assert.equal(segments[0].folds[0].resultLive, false)
})

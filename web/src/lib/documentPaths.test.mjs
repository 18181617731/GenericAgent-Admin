import test from 'node:test'
import assert from 'node:assert/strict'
import { extractDocumentPaths, isDocumentPath, isMarkdownDocumentPath } from './documentPaths.js'

test('extracts standalone paths from result code blocks and explicit labels', () => {
    const content = [
      '已合并至：',
      '',
      '```text',
      String.raw`G:\MygenericAgent\temp\projects\闲鱼\综合报告.md`,
      '```',
      '',
      `Report: ${String.raw`C:\tmp\final-report.txt`}`,
    ].join('\n')

    assert.deepEqual(extractDocumentPaths(content), [
      String.raw`G:\MygenericAgent\temp\projects\闲鱼\综合报告.md`,
      String.raw`C:\tmp\final-report.txt`,
    ])
})

test('does not turn tool code, URLs, or FILE markers into duplicate result cards', () => {
    const content = [
      'Tool: code_run',
      '```python',
      String.raw`path = r"G:\workspace\internal\notes.md"`,
      String.raw`url = "https://example.com/report.md"`,
      '```',
      String.raw`[FILE:C:\tmp\report.md]`,
    ].join('\n')

    assert.deepEqual(extractDocumentPaths(content), [])
})

test('accepts supported document paths and identifies Markdown files', () => {
  assert.equal(isDocumentPath(String.raw`D:\reports\summary.pdf`), true)
  assert.equal(isDocumentPath('/mnt/data/private.md'), false)
  assert.equal(isDocumentPath('https://example.com/readme.md'), false)
  assert.equal(isMarkdownDocumentPath(String.raw`D:\reports\summary.MD`), true)
  assert.equal(isMarkdownDocumentPath(String.raw`D:\reports\summary.txt`), false)
})

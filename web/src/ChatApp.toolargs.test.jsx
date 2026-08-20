import { describe, expect, test } from 'vitest'
import { parseToolReceiptArgs, parseToolResultDetails } from './ChatApp.jsx'

describe('tool receipt argument parsing', () => {
  test('parses valid JSON without changing typed values or escaped code', () => {
    expect(parseToolReceiptArgs('{"script":"print(\\"ok\\")\\nnext","count":2,"inline_eval":false}')).toEqual({
      script: 'print("ok")\nnext',
      count: 2,
      inline_eval: false,
    })
  })

  test('repairs literal control characters inside JSON strings', () => {
    const body = '{"script":"import time\ntime.sleep(15)\nprint(\\"done\\")","content":"a\tb\rc"}'
    expect(parseToolReceiptArgs(body)).toEqual({
      script: 'import time\ntime.sleep(15)\nprint("done")',
      content: 'a\tb\rc',
    })
  })

  test('preserves Windows paths whose backslashes were not JSON-escaped', () => {
    expect(parseToolReceiptArgs(String.raw`{"path":"E:\temp\new.txt"}`)).toEqual({
      path: String.raw`E:\temp\new.txt`,
    })
    expect(parseToolReceiptArgs(String.raw`{"path":"E:\\temp\\new.txt"}`)).toEqual({
      path: String.raw`E:\temp\new.txt`,
    })
  })

  test('falls back for malformed or non-object JSON', () => {
    expect(parseToolReceiptArgs('{"script":')).toEqual({})
    expect(parseToolReceiptArgs('[1,2]')).toEqual({})
  })
})

describe('tool receipt result parsing', () => {
  test('splits action, status and output while preserving multiline content', () => {
    expect(parseToolResultDetails('[Action] Running python in temp: import time\r\ntime.sleep(15)\r\nprint("done")\r\n[Status] \u2705 Exit Code: 0\r\n[Stdout]\r\ndone\r\n')).toEqual([
      { kind: 'action', content: 'Running python in temp: import time\ntime.sleep(15)\nprint("done")' },
      { kind: 'status', content: '\u2705 Exit Code: 0' },
      { kind: 'stdout', content: 'done' },
    ])
  })

  test('keeps stderr and empty stdout sections in protocol order', () => {
    expect(parseToolResultDetails('[Action] run\n[Status] \u274c Exit Code: 1\n[Stdout]\n[Stderr]\nboom')).toEqual([
      { kind: 'action', content: 'run' },
      { kind: 'status', content: '\u274c Exit Code: 1' },
      { kind: 'stdout', content: '' },
      { kind: 'stderr', content: 'boom' },
    ])
  })

  test('falls back when text does not use recognized result markers', () => {
    expect(parseToolResultDetails('plain tool output')).toBeNull()
    expect(parseToolResultDetails('[Stdout]\nonly output')).toBeNull()
    expect(parseToolResultDetails('prefix\n[Status] ok')).toBeNull()
  })
})

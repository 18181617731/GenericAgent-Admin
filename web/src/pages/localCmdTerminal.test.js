import { describe, expect, test } from 'vitest'
import { appendTerminalText } from './localCmdTerminal.js'

describe('remote CMD terminal output', () => {
  test('keeps a CRLF split across output chunks as one newline', () => {
    const state = { pendingCR: false }
    const first = appendTerminalText('', '中文_CMD_OK\r', state)
    const output = appendTerminalText(first, '\nD:\\中文 local cmd>', state)

    expect(output).toBe('中文_CMD_OK\nD:\\中文 local cmd>')
    expect(state.pendingCR).toBe(false)
  })

  test('separates a prompt after ConPTY cursor positioning', () => {
    const raw = '\x1b[?25lecho 中文_CMD_OK\r\n中文_CMD_OK\x1b[13;1HC:\\workspace_ai\\GenericAgent-Admin>\x1b[15;1H\x1b[?25h'
    const output = appendTerminalText('', raw, { pendingCR: false })

    expect(output).toContain('中文_CMD_OK\nC:\\workspace_ai\\GenericAgent-Admin>')
    expect(output).not.toContain('中文_CMD_OKC:\\workspace_ai')
  })
})

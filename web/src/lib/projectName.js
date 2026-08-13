// Mirrors validProjectModeName in internal/api/chat_handlers.go. A project name
// becomes a directory under {GA_ROOT}/temp/projects, so it has to be one safe
// path segment. Checking it here turns a 400 into feedback while typing; the
// server still validates, and stays the authority.

const CONTROL_CHARS = /[\u0000-\u001f\u007f]/
const SEPARATORS = /[\\/:]/
const MAX_BYTES = 128

export const projectNameError = (raw) => {
  const name = String(raw ?? '').trim()
  if (!name) return 'empty'
  if (name === '.' || name === '..') return 'reserved'
  if (SEPARATORS.test(name)) return 'separator'
  if (CONTROL_CHARS.test(name)) return 'control'
  if (name.endsWith('.')) return 'trailing_dot'
  if (new TextEncoder().encode(name).length > MAX_BYTES) return 'too_long'
  return ''
}

export const isValidProjectName = (raw) => !projectNameError(raw)

export const projectNameErrorText = (code, translate) => {
  switch (code) {
    case 'empty':
      return translate('请输入项目名。', 'Enter a project name.')
    case 'separator':
      return translate('项目名不能包含 / \\ 或 : 。', 'A project name cannot contain / \\ or :.')
    case 'reserved':
    case 'trailing_dot':
      return translate('项目名不能是 . 或 .. ，也不能以 . 结尾。', 'A project name cannot be . or .., and cannot end with a dot.')
    case 'control':
      return translate('项目名不能包含控制字符。', 'A project name cannot contain control characters.')
    case 'too_long':
      return translate('项目名太长，请控制在 128 字节以内。', 'That project name is too long; keep it under 128 bytes.')
    default:
      return ''
  }
}

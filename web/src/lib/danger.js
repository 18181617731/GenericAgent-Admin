export const confirmDanger = (operation, message, confirmFn = globalThis.window?.confirm) => {
  const op = operation ? `[${operation}] ` : ''
  const text = `${op}${message || 'Confirm dangerous operation?'}`
  if (typeof confirmFn !== 'function') return false
  return confirmFn(text)
}

export const isDangerDisabled = (...flags) => flags.some(Boolean)

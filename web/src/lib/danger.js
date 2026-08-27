let dialogAdapter = null
const pendingDialogs = []

const currentLocale = () => {
  const lang = globalThis.document?.documentElement?.lang?.toLowerCase() || ''
  return lang.startsWith('en') ? 'en' : 'zh'
}

const deliver = ({ request, resolve }) => {
  let result
  try {
    result = dialogAdapter(request)
  } catch {
    resolve(false)
    return
  }
  Promise.resolve(result).then(
    value => resolve(Boolean(value)),
    () => resolve(false),
  )
}

export const registerDialogAdapter = adapter => {
  if (typeof adapter !== 'function') throw new TypeError('Dialog adapter must be a function')
  dialogAdapter = adapter
  pendingDialogs.splice(0).forEach(deliver)
  return () => {
    if (dialogAdapter === adapter) dialogAdapter = null
  }
}

const requestDialog = request => new Promise(resolve => {
  const entry = { request, resolve }
  if (dialogAdapter) deliver(entry)
  else pendingDialogs.push(entry)
})

export const confirmDanger = (operation, message) => requestDialog({
  kind: 'confirm',
  locale: currentLocale(),
  operation: operation || '',
  message: String(message || 'Confirm dangerous operation?'),
})

export const showAppAlert = (message, options = {}) => requestDialog({
  kind: 'alert',
  locale: currentLocale(),
  message: String(message || ''),
  title: options.title || '',
  operation: options.operation || '',
  confirmLabel: options.confirmLabel || '',
})

export const isDangerDisabled = (...flags) => flags.some(Boolean)

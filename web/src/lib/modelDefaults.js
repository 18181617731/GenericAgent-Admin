const numericModelIndex = value => {
  const index = Number(value?.index ?? value)
  return Number.isInteger(index) && index >= 0 ? index : null
}

export const orderedRuntimeModels = (models = []) => (Array.isArray(models) ? models : [])
  .map((model, position) => ({ model, position, index: numericModelIndex(model) }))
  .sort((left, right) => {
    if (left.index === null && right.index === null) return left.position - right.position
    if (left.index === null) return 1
    if (right.index === null) return -1
    return left.index - right.index || left.position - right.position
  })
  .map(item => item.model)

export const firstRuntimeModel = (models = []) => orderedRuntimeModels(models)[0] || null

export const firstRuntimeModelNo = (models = [], fallback = 0) => {
  const first = firstRuntimeModel(models)
  const firstIndex = first ? numericModelIndex(first) : null
  if (firstIndex !== null) return firstIndex
  const safeFallback = numericModelIndex({ index: fallback })
  return safeFallback === null ? 0 : safeFallback
}

export const modelDisplayName = (model, fallback = '') => {
  const display = String(model?.display_name || model?.displayName || model?.label || model?.name || model?.model || '').trim()
  return display || fallback
}

export const runtimeModelProvider = (model, fallback = '未分组服务商') => {
  const provider = String(model?.provider || model?.providerName || model?.provider_name || '').trim()
  if (provider) return provider
  const name = String(model?.name || '').trim()
  const modelName = String(model?.model || '').trim()
  if (name && modelName && name.endsWith(`/${modelName}`)) return name.slice(0, -(modelName.length + 1))
  const split = name.lastIndexOf('/')
  return (split > 0 ? name.slice(0, split) : name) || fallback
}

export const runtimeModelDescription = (model, fallback = '未配置模型') => {
  const provider = runtimeModelProvider(model, '')
  const display = modelDisplayName(model, fallback)
  const index = numericModelIndex(model)
  return [provider, display, index === null ? '' : `#${index}`].filter(Boolean).join(' · ')
}

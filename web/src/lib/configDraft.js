const CONFIG_FIELDS = [
  'python_path',
  'chat_data_dir',
  'proxy_mode',
  'http_proxy',
  'https_proxy',
  'all_proxy',
  'no_proxy',
]

export const configDraftSnapshot = (root = '', config = {}) => Object.fromEntries([
  ['ga_root', String(root || '')],
  ...CONFIG_FIELDS.map(field => [field, String(config?.[field] || (field === 'proxy_mode' ? 'off' : ''))]),
])

export const configDraftDirty = (root, config, persistedConfig) => {
  if (!persistedConfig) return false
  return JSON.stringify(configDraftSnapshot(root, config)) !== JSON.stringify(configDraftSnapshot(persistedConfig.ga_root, persistedConfig))
}

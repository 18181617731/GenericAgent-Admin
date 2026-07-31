import { useMemo } from 'react'
import { ProviderModelCascade, buildModelProviderGroups, findModelProviderValue } from './ModelProviderCascade.jsx'

export function ModelCascadePicker({
  models = [],
  groups: providedGroups,
  value,
  onChange,
  disabled = false,
  allowDefault = false,
  defaultLabel = '使用默认模型',
  label = '模型',
  className = '',
  placement = 'top',
  align = 'end',
  mobile,
}) {
  const groups = useMemo(() => providedGroups || buildModelProviderGroups(models, {
    defaultLabel: allowDefault ? defaultLabel : '',
  }), [allowDefault, defaultLabel, models, providedGroups])
  const selectedProvider = findModelProviderValue(groups, value)

  return <ProviderModelCascade
    groups={groups}
    selectedProvider={selectedProvider}
    value={value}
    onChange={onChange}
    disabled={disabled || (!allowDefault && !groups.length)}
    mobile={mobile}
    placement={placement === 'auto' ? 'auto' : placement}
    align={align}
    label={label}
    showLabel={Boolean(label)}
    className={`model-cascade-picker ${className}`.trim()}
  />
}

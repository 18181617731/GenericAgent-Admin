import { useEffect, useMemo, useRef, useState } from 'react'
import { Check, ChevronDown, ChevronRight } from 'lucide-react'
import { groupRuntimeModels } from '../lib/format'

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
}) {
  const groups = useMemo(() => providedGroups || groupRuntimeModels(models), [providedGroups, models])
  const selectedGroup = groups.find(group => group.models.some(model => String(model.value) === String(value)))
  const [open, setOpen] = useState(false)
  const [previewProvider, setPreviewProvider] = useState(selectedGroup?.value || groups[0]?.value || '')
  const ref = useRef(null)
  const openedAt = useRef(0)

  useEffect(() => {
    if (!open) return undefined
    const close = event => { if (!ref.current?.contains(event.target)) setOpen(false) }
    const closeOnEscape = event => { if (event.key === 'Escape') setOpen(false) }
    const closeOnScroll = event => {
      if (performance.now() - openedAt.current < 180) return
      if (!ref.current?.contains(event.target)) setOpen(false)
    }
    document.addEventListener('mousedown', close)
    document.addEventListener('keydown', closeOnEscape)
    window.addEventListener('scroll', closeOnScroll, true)
    return () => {
      document.removeEventListener('mousedown', close)
      document.removeEventListener('keydown', closeOnEscape)
      window.removeEventListener('scroll', closeOnScroll, true)
    }
  }, [open])

  useEffect(() => {
    if (selectedGroup) setPreviewProvider(selectedGroup.value)
    else if (groups[0]) setPreviewProvider(groups[0].value)
    else setPreviewProvider('')
  }, [selectedGroup, groups])

  const previewGroup = groups.find(group => group.value === previewProvider) || selectedGroup || groups[0]
  const activeModel = selectedGroup?.models.find(model => String(model.value) === String(value))
  const usingDefault = allowDefault && (value === '' || value === null || value === undefined)
  const displayModel = usingDefault ? defaultLabel : (activeModel?.label || '请选择模型')

  return <div className={`model-cascade-picker oa-composer-cascade placement-${placement} ${className}`.trim()} ref={ref}>
    {label && <span className="model-cascade-label">{label}</span>}
    <button type="button" disabled={disabled || (!allowDefault && !groups.length)} title={displayModel} aria-label={`${label || '模型'}：${displayModel}`} aria-expanded={open} onClick={() => { openedAt.current = performance.now(); setOpen(current => !current) }}>
      <span className="oa-cascade-current-model">{displayModel}</span>
      <ChevronDown size={13}/>
    </button>
    {open && <div className="oa-cascade-menu" role="dialog" aria-label="服务商和模型">
      <div className="oa-cascade-providers">
        {allowDefault && <button type="button" className={usingDefault ? 'active' : ''} onClick={() => { onChange(''); setOpen(false) }}><span>{defaultLabel}</span>{usingDefault && <Check size={12}/>}</button>}
        {groups.map(group => <button key={group.value} type="button" className={group.value === selectedGroup?.value ? 'active' : ''} onMouseEnter={() => setPreviewProvider(group.value)} onFocus={() => setPreviewProvider(group.value)} onClick={() => setPreviewProvider(group.value)}>
          <span>{group.label}</span><ChevronRight size={13}/>
        </button>)}
      </div>
      <div className="oa-cascade-models">
        <div className="oa-cascade-heading">{previewGroup?.label || '模型'}</div>
        {previewGroup?.models.length ? previewGroup.models.map(model => <button key={model.value} type="button" className={String(model.value) === String(value) ? 'active' : ''} onClick={() => { onChange(model.value); setOpen(false) }}>
          {String(model.value) === String(value) && <Check size={12}/>}<span>{model.label}</span>
        </button>) : <div className="oa-cascade-empty">未发现模型</div>}
      </div>
    </div>}
  </div>
}

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown, ChevronRight, X } from 'lucide-react'
import { modelLabel } from '../lib/format'

const DEFAULT_PROVIDER_VALUE = '__ga_default_provider__'

export const modelProvider = model => {
  const provider = String(model?.provider || '').trim()
  if (provider) return provider
  const name = String(model?.name || '').trim()
  const modelName = String(model?.model || '').trim()
  if (name && modelName && name.endsWith(`/${modelName}`)) return name.slice(0, -(modelName.length + 1))
  const split = name.lastIndexOf('/')
  return (split > 0 ? name.slice(0, split) : name) || '未分组服务商'
}

export const runtimeModelLabel = model => {
  const modelName = String(model?.model || '').trim()
  if (modelName) return modelName
  const label = modelLabel(model)
  return label.includes('/') ? label.split('/').pop() : label
}

export function buildModelProviderGroups(models = [], options = {}) {
  const { defaultLabel = '', defaultProviderLabel = '默认' } = options
  const groups = new Map()
  models.forEach(model => {
    const provider = modelProvider(model)
    if (!groups.has(provider)) groups.set(provider, [])
    groups.get(provider).push({ value: model.index, label: runtimeModelLabel(model) })
  })
  const result = Array.from(groups, ([provider, providerModels]) => ({ value: provider, label: provider, models: providerModels }))
  if (defaultLabel) result.unshift({ value: DEFAULT_PROVIDER_VALUE, label: defaultProviderLabel, models: [{ value: '', label: defaultLabel }] })
  return result
}

export function findModelProviderValue(groups = [], value) {
  return groups.find(group => group.models.some(model => String(model.value) === String(value)))?.value || ''
}

function ProviderModelMenu({ groups, selectedProvider, previewProvider, value, onPreview, onSelect, onClose, mobile }) {
  const previewGroup = groups.find(group => group.value === previewProvider) || groups[0]
  return <>
    {mobile && <div className="oa-mobile-picker-head"><div><b>选择模型</b><span>先选择服务商，再选择模型</span></div><button type="button" onClick={onClose} aria-label="关闭模型选择"><X size={18}/></button></div>}
    <div className="oa-cascade-providers">
      {groups.map(group => <button key={group.value} type="button" className={group.value === previewGroup?.value ? 'active' : ''}
        onMouseEnter={() => onPreview(group.value)} onFocus={() => onPreview(group.value)} onClick={() => onPreview(group.value)}>
        <span>{group.label}</span><ChevronRight size={13}/>
      </button>)}
    </div>
    <div className="oa-cascade-models">
      <div className="oa-cascade-heading">{previewGroup?.label || '模型'}</div>
      {previewGroup?.models.length ? previewGroup.models.map(model => <button key={String(model.value)} type="button"
        className={previewGroup.value === selectedProvider && String(model.value) === String(value) ? 'active' : ''}
        onClick={() => onSelect(model.value)}>
        {previewGroup.value === selectedProvider && String(model.value) === String(value) && <Check size={12}/>}<span>{model.label}</span>
      </button>) : <div className="oa-cascade-empty">未发现模型</div>}
    </div>
  </>
}

export function ProviderModelCascade({
  groups = [],
  selectedProvider = '',
  value,
  onChange,
  disabled = false,
  mobile,
  className = '',
  label = '模型',
  showLabel = true,
  placement = 'top',
  align = 'end',
}) {
  const [open, setOpen] = useState(false)
  const [previewProvider, setPreviewProvider] = useState(selectedProvider || groups[0]?.value || '')
  const [detectedMobile, setDetectedMobile] = useState(false)
  const [resolvedPlacement, setResolvedPlacement] = useState(placement === 'auto' ? 'bottom' : placement)
  const ref = useRef()
  const menuRef = useRef()
  const openedAt = useRef(0)
  const mobileMode = mobile ?? detectedMobile

  useEffect(() => {
    if (mobile !== undefined || typeof window === 'undefined' || !window.matchMedia) return undefined
    const media = window.matchMedia('(max-width: 560px)')
    const sync = () => setDetectedMobile(media.matches)
    sync()
    media.addEventListener?.('change', sync)
    media.addListener?.(sync)
    return () => {
      media.removeEventListener?.('change', sync)
      media.removeListener?.(sync)
    }
  }, [mobile])

  useEffect(() => {
    if (!open) return undefined
    const close = () => setOpen(false)
    const isInside = target => typeof Node !== 'undefined' && target instanceof Node && (ref.current?.contains(target) || menuRef.current?.contains(target))
    const onMouseDown = event => { if (!isInside(event.target)) close() }
    const onKeyDown = event => { if (event.key === 'Escape') close() }
    const onScroll = event => {
      if (performance.now() - openedAt.current < 250) return
      if (!isInside(event.target)) close()
    }
    document.addEventListener('mousedown', onMouseDown)
    document.addEventListener('keydown', onKeyDown)
    window.addEventListener('scroll', onScroll, true)
    return () => {
      document.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('scroll', onScroll, true)
    }
  }, [open, mobileMode])

  useEffect(() => {
    const next = selectedProvider || findModelProviderValue(groups, value) || groups[0]?.value || ''
    setPreviewProvider(next)
  }, [selectedProvider, groups, value])

  useLayoutEffect(() => {
    if (!open || mobileMode || placement !== 'auto' || !ref.current || !menuRef.current) {
      setResolvedPlacement(placement === 'auto' ? 'bottom' : placement)
      return
    }
    const triggerBox = ref.current.getBoundingClientRect()
    const menuHeight = menuRef.current.getBoundingClientRect().height
    const spaceAbove = triggerBox.top - 12
    const spaceBelow = window.innerHeight - triggerBox.bottom - 12
    setResolvedPlacement(spaceBelow >= menuHeight || spaceBelow >= spaceAbove ? 'bottom' : 'top')
  }, [open, mobileMode, placement, groups, previewProvider])

  const resolvedProvider = selectedProvider || findModelProviderValue(groups, value) || groups[0]?.value || ''
  const activeGroup = groups.find(group => group.value === resolvedProvider)
  const activeModel = activeGroup?.models.find(model => String(model.value) === String(value))
  const displayModel = activeModel?.label || '未发现模型'
  const displayTitle = [activeGroup?.label, displayModel].filter(Boolean).join(' · ')
  const selectModel = next => { onChange?.(next); setOpen(false) }
  const menuClass = `oa-cascade-menu ${mobileMode ? 'oa-cascade-modal' : ''} is-placement-${resolvedPlacement} is-align-${align}`
  const menu = <div className={menuClass} ref={menuRef} role="dialog" aria-modal={mobileMode || undefined} aria-label="服务商和模型">
    <ProviderModelMenu groups={groups} selectedProvider={resolvedProvider} previewProvider={previewProvider} value={value}
      onPreview={setPreviewProvider} onSelect={selectModel} onClose={() => setOpen(false)} mobile={mobileMode}/>
  </div>

  return <>
    <div className={`oa-model-select oa-composer-cascade ${className}`.trim()} ref={ref}>
      {showLabel && <span>{label}</span>}
      <button type="button" disabled={disabled} title={displayTitle} aria-label={`选择模型，当前 ${displayTitle || displayModel}`} aria-expanded={open} onClick={() => {
        openedAt.current = performance.now()
        setOpen(current => !current)
      }}>
        <span className="oa-cascade-current-model">{displayModel}</span><ChevronDown size={13}/>
      </button>
      {open && !mobileMode && menu}
    </div>
    {open && mobileMode && createPortal(<div className="oa-mobile-picker-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setOpen(false) }}>{menu}</div>, document.body)}
  </>
}

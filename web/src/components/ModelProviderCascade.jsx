import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown, ChevronRight, Search, X } from 'lucide-react'
import { modelLabel } from '../lib/format'
import { filterModelProviderGroups, modelGroupStats } from '../lib/ux'

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

function focusMenuItem(container, selector, current, offset) {
  const items = Array.from(container?.querySelectorAll(selector) || [])
  if (!items.length) return
  const index = Math.max(0, items.indexOf(current))
  items[(index + offset + items.length) % items.length]?.focus()
}

function ProviderModelMenu({ groups, selectedProvider, previewProvider, value, onPreview, onSelect, onClose, mobile, query, onQuery }) {
  const previewGroup = groups.find(group => group.value === previewProvider) || groups[0]
  const stats = modelGroupStats(groups)
  const onListKeyDown = event => {
    const provider = event.target.closest?.('[data-cascade-provider]')
    const model = event.target.closest?.('[data-cascade-model]')
    if (!provider && !model) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      focusMenuItem(event.currentTarget, provider ? '[data-cascade-provider]' : '[data-cascade-model]', event.target, event.key === 'ArrowDown' ? 1 : -1)
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault()
      const items = event.currentTarget.querySelectorAll(provider ? '[data-cascade-provider]' : '[data-cascade-model]')
      items[event.key === 'Home' ? 0 : items.length - 1]?.focus()
    } else if (event.key === 'ArrowRight' && provider) {
      event.preventDefault()
      event.currentTarget.querySelector('[data-cascade-model]')?.focus()
    } else if (event.key === 'ArrowLeft' && model) {
      event.preventDefault()
      Array.from(event.currentTarget.querySelectorAll('[data-cascade-provider]'))
        .find(item => item.dataset.providerValue === String(previewGroup?.value || ''))?.focus()
    }
  }
  return <>
    <div className={`oa-mobile-picker-head ${mobile ? '' : 'oa-cascade-head'}`}>
      <div><b>选择模型</b><span>{stats.providers} 个服务商 · {stats.models} 个模型</span></div>
      {mobile && <button type="button" onClick={onClose} aria-label="关闭模型选择"><X size={18}/></button>}
    </div>
    <label className="oa-cascade-search"><Search size={14} aria-hidden="true"/><input value={query} onChange={event => onQuery(event.target.value)} onKeyDown={event => {
      if (event.key === 'ArrowDown') {
        event.preventDefault()
        event.currentTarget.closest('.oa-cascade-menu')?.querySelector('[data-cascade-provider], [data-cascade-model]')?.focus()
      }
    }} placeholder="搜索服务商或模型" aria-label="搜索服务商或模型"/></label>
    <div className="oa-cascade-body" onKeyDown={onListKeyDown}>
    <div className="oa-cascade-providers" role="listbox" aria-label="服务商">
      {groups.map(group => <button key={group.value} type="button" role="option" aria-label={group.label} aria-selected={group.value === previewGroup?.value} data-cascade-provider data-provider-value={String(group.value)} className={group.value === previewGroup?.value ? 'active' : ''}
        onMouseEnter={() => onPreview(group.value)} onFocus={() => onPreview(group.value)} onClick={() => onPreview(group.value)}>
        <span>{group.label}</span><small>{group.models.length}</small><ChevronRight size={13}/>
      </button>)}
    </div>
    <div className="oa-cascade-models" role="listbox" aria-label={previewGroup ? `${previewGroup.label} 模型` : '模型'}>
      <div className="oa-cascade-heading">{previewGroup?.label || '模型'}</div>
      {previewGroup?.models.length ? previewGroup.models.map(model => <button key={String(model.value)} type="button" role="option" aria-selected={previewGroup.value === selectedProvider && String(model.value) === String(value)} data-cascade-model
        className={previewGroup.value === selectedProvider && String(model.value) === String(value) ? 'active' : ''}
        onClick={() => onSelect(model.value)}>
        {previewGroup.value === selectedProvider && String(model.value) === String(value) && <Check size={12}/>}<span>{model.label}</span>
      </button>) : <div className="oa-cascade-empty" role="status"><b>{query ? '没有匹配的模型' : '尚未配置可用模型'}</b><span>{query ? '请尝试模型 ID 或服务商名称中的其他关键词。' : '请前往“模型”页面添加并保存服务商配置。'}</span></div>}
    </div>
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
  disabledReason = '',
}) {
  const [open, setOpen] = useState(false)
  const [previewProvider, setPreviewProvider] = useState(selectedProvider || groups[0]?.value || '')
  const [query, setQuery] = useState('')
  const [detectedMobile, setDetectedMobile] = useState(false)
  const [resolvedPlacement, setResolvedPlacement] = useState(placement === 'auto' ? 'bottom' : placement)
  const ref = useRef()
  const triggerRef = useRef()
  const menuRef = useRef()
  const openedAt = useRef(0)
  const mobileMode = mobile ?? detectedMobile
  const filteredGroups = useMemo(() => filterModelProviderGroups(groups, query), [groups, query])

  const closeMenu = useCallback((restoreFocus = true) => {
    setOpen(false)
    setQuery('')
    if (restoreFocus) window.requestAnimationFrame(() => triggerRef.current?.focus())
  }, [])

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
    const isInside = target => typeof Node !== 'undefined' && target instanceof Node && (ref.current?.contains(target) || menuRef.current?.contains(target))
    const onMouseDown = event => { if (!isInside(event.target)) closeMenu() }
    const onKeyDown = event => { if (event.key === 'Escape') { event.preventDefault(); closeMenu() } }
    const onScroll = event => {
      if (mobileMode) return
      if (performance.now() - openedAt.current < 250) return
      if (!isInside(event.target)) closeMenu()
    }
    document.addEventListener('mousedown', onMouseDown)
    document.addEventListener('keydown', onKeyDown)
    window.addEventListener('scroll', onScroll, true)
    return () => {
      document.removeEventListener('mousedown', onMouseDown)
      document.removeEventListener('keydown', onKeyDown)
      window.removeEventListener('scroll', onScroll, true)
    }
  }, [open, mobileMode, closeMenu])

  useEffect(() => {
    if (!open) return undefined
    const previousOverflow = document.body.style.overflow
    if (mobileMode) document.body.style.overflow = 'hidden'
    const frame = window.requestAnimationFrame(() => menuRef.current?.querySelector('.oa-cascade-search input')?.focus())
    return () => {
      window.cancelAnimationFrame(frame)
      if (mobileMode) document.body.style.overflow = previousOverflow
    }
  }, [open, mobileMode])

  useEffect(() => {
    const next = selectedProvider || findModelProviderValue(groups, value) || groups[0]?.value || ''
    setPreviewProvider(next)
  }, [selectedProvider, groups, value])

  useEffect(() => {
    if (!filteredGroups.some(group => group.value === previewProvider)) setPreviewProvider(filteredGroups[0]?.value || '')
  }, [filteredGroups, previewProvider])

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
  const selectModel = next => { onChange?.(next); closeMenu() }
  const menuClass = `oa-cascade-menu ${mobileMode ? 'oa-cascade-modal' : ''} is-placement-${resolvedPlacement} is-align-${align}`
  const menu = <div className={menuClass} ref={menuRef} role="dialog" aria-modal={mobileMode || undefined} aria-label="服务商和模型">
    <ProviderModelMenu groups={filteredGroups} selectedProvider={resolvedProvider} previewProvider={previewProvider} value={value} query={query} onQuery={setQuery}
      onPreview={setPreviewProvider} onSelect={selectModel} onClose={closeMenu} mobile={mobileMode}/>
  </div>

  return <>
    <div className={`oa-model-select oa-composer-cascade ${className}`.trim()} ref={ref}>
      {showLabel && <span>{label}</span>}
      <button ref={triggerRef} type="button" disabled={disabled} title={disabled && disabledReason ? disabledReason : displayTitle} aria-label={`选择模型，当前 ${displayTitle || displayModel}${disabled && disabledReason ? `，${disabledReason}` : ''}`} aria-haspopup="dialog" aria-expanded={open} onClick={() => {
        openedAt.current = performance.now()
        setOpen(current => !current)
      }}>
        <span className="oa-cascade-current-model">{displayModel}</span><ChevronDown size={13}/>
      </button>
      {open && !mobileMode && menu}
    </div>
    {open && mobileMode && createPortal(<div className="oa-mobile-picker-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) closeMenu() }}>{menu}</div>, document.body)}
  </>
}

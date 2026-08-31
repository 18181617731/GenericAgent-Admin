import React, { useId, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Check, ChevronDown, Palette, X } from 'lucide-react'
import { getTheme, THEMES } from './themes'

const copy = {
  zh: {
    trigger: '\u5916\u89c2',
    title: '\u9009\u62e9\u5916\u89c2',
    hint: '\u76f4\u63a5\u9009\u62e9\u4e00\u5957\u4e3b\u9898',
    close: '\u5173\u95ed\u4e3b\u9898\u9009\u62e9\u5668',
  },
  en: {
    trigger: 'Appearance',
    title: 'Choose appearance',
    hint: 'Select any theme directly',
    close: 'Close theme picker',
  },
}

const clamp = (value, min, max) => Math.min(Math.max(value, min), max)

export default function ThemePicker({ value, onChange, lang = 'en', variant = 'default', className = '' }) {
  const [open, setOpen] = useState(false)
  const [position, setPosition] = useState({ left: 12, top: 12, ready: false })
  const triggerRef = useRef(null)
  const panelRef = useRef(null)
  const titleId = useId()
  const text = copy[lang] || copy.en
  const activeTheme = getTheme(value)

  useLayoutEffect(() => {
    if (!open) return undefined
    const place = () => {
      const anchor = triggerRef.current?.getBoundingClientRect()
      const panel = panelRef.current
      if (!anchor || !panel) return
      const gap = 8
      const margin = 12
      const width = panel.offsetWidth || 340
      const height = panel.offsetHeight || 320
      const left = clamp(anchor.right - width, margin, window.innerWidth - width - margin)
      const top = anchor.bottom + gap + height <= window.innerHeight - margin
        ? anchor.bottom + gap
        : Math.max(margin, anchor.top - height - gap)
      setPosition({ left, top, ready: true })
    }
    place()
    window.addEventListener('resize', place)
    window.addEventListener('scroll', place, true)
    return () => {
      window.removeEventListener('resize', place)
      window.removeEventListener('scroll', place, true)
    }
  }, [open])

  useLayoutEffect(() => {
    if (!open) return undefined
    const dismiss = event => {
      if (triggerRef.current?.contains(event.target) || panelRef.current?.contains(event.target)) return
      setOpen(false)
    }
    const onKeyDown = event => {
      if (event.key !== 'Escape') return
      event.preventDefault()
      setOpen(false)
      triggerRef.current?.focus()
    }
    document.addEventListener('pointerdown', dismiss)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', dismiss)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  const choose = themeId => onChange?.(themeId)
  const navigateOptions = (event, index) => {
    const keys = ['ArrowDown', 'ArrowRight', 'ArrowUp', 'ArrowLeft', 'Home', 'End']
    if (!keys.includes(event.key)) return
    event.preventDefault()
    let next = index
    if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = THEMES.length - 1
    else next = (index + (event.key === 'ArrowDown' || event.key === 'ArrowRight' ? 1 : -1) + THEMES.length) % THEMES.length
    choose(THEMES[next].id)
    panelRef.current?.querySelectorAll('[role="radio"]')[next]?.focus()
  }

  const panel = open && createPortal(
    <section
      ref={panelRef}
      className="theme-picker-panel"
      role="dialog"
      aria-modal="false"
      aria-labelledby={titleId}
      style={{ left: position.left, top: position.top, visibility: position.ready ? 'visible' : 'hidden' }}
    >
      <header className="theme-picker-head">
        <span className="theme-picker-heading">
          <b id={titleId}>{text.title}</b>
          <small>{text.hint}</small>
        </span>
        <button type="button" className="theme-picker-close" aria-label={text.close} onClick={() => setOpen(false)}><X size={15}/></button>
      </header>
      <div className="theme-picker-options" role="radiogroup" aria-labelledby={titleId}>
        {THEMES.map((theme, index) => {
          const selected = theme.id === activeTheme.id
          const Icon = theme.icon
          const label = theme.label[lang] || theme.label.en
          const description = theme.description?.[lang] || theme.description?.en || ''
          return <button
            key={theme.id}
            type="button"
            role="radio"
            aria-checked={selected}
            className={`theme-picker-option ${selected ? 'is-selected' : ''}`}
            onClick={() => choose(theme.id)}
            onKeyDown={event => navigateOptions(event, index)}
          >
            <span className="theme-picker-preview" aria-hidden="true">
              {theme.preview.map((color, colorIndex) => <i key={colorIndex} style={{ background: color }}/>) }
            </span>
            <span className="theme-picker-option-copy">
              <span><Icon size={15}/><b>{label}</b></span>
              <small>{description}</small>
            </span>
            <span className="theme-picker-check" aria-hidden="true">{selected && <Check size={15}/>}</span>
          </button>
        })}
      </div>
    </section>,
    document.body,
  )

  return <div className={`theme-picker theme-picker--${variant} ${className}`.trim()}>
    <button
      ref={triggerRef}
      type="button"
      className="theme-picker-trigger"
      aria-label={`${text.trigger}: ${activeTheme.label[lang] || activeTheme.label.en}`}
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-label={`${text.title}: ${activeTheme.label[lang] || activeTheme.label.en}`}
      title={variant === 'compact' ? `${text.title}: ${activeTheme.label[lang] || activeTheme.label.en}` : undefined}
      onClick={() => setOpen(current => !current)}
    >
      <span className="theme-picker-trigger-icon"><Palette size={16}/></span>
      <span className="theme-picker-trigger-copy">
        <small>{text.trigger}</small>
        <b>{activeTheme.label[lang] || activeTheme.label.en}</b>
      </span>
      <span className="theme-picker-trigger-preview" aria-hidden="true">
        {activeTheme.preview.map((color, index) => <i key={index} style={{ background: color }}/>) }
      </span>
      <ChevronDown className="theme-picker-chevron" size={15}/>
    </button>
    {panel}
  </div>
}

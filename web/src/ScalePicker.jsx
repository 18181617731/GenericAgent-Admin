import { Minus, Plus, RotateCcw, ZoomIn } from 'lucide-react'
import { formatUIScale, normalizeUIScale, stepUIScale, UI_SCALE_MAX, UI_SCALE_MIN } from './lib/uiScale.js'

const copy = {
  zh: {
    title: '界面缩放',
    hint: '调整字体、间距和组件大小',
    decrease: '缩小界面',
    increase: '放大界面',
    reset: '恢复默认 100%',
    keyboard: '快捷键：Ctrl/Cmd + 加号或减号调整，Ctrl/Cmd + 0 恢复默认。',
  },
  en: {
    title: 'Interface scale',
    hint: 'Adjust text, spacing, and component size',
    decrease: 'Decrease interface scale',
    increase: 'Increase interface scale',
    reset: 'Reset to 100%',
    keyboard: 'Shortcuts: Ctrl/Cmd + plus or minus to adjust; Ctrl/Cmd + 0 to reset.',
  },
}

export function ScalePicker({ value, onChange, lang = 'en', variant = 'default', className = '' }) {
  const text = copy[lang] || copy.en
  const scale = normalizeUIScale(value)
  const change = direction => onChange?.(stepUIScale(scale, direction))
  const reset = () => onChange?.(1)

  return <div className={`ui-scale-picker ui-scale-picker--${variant} ${className}`.trim()}>
    <span className="ui-scale-label"><ZoomIn size={15} aria-hidden="true"/><span>{text.title}</span></span>
    <div className="ui-scale-controls" role="group" aria-label={text.title}>
      <button type="button" className="ui-scale-button" onClick={() => change(-1)} disabled={scale <= UI_SCALE_MIN} aria-label={text.decrease} title={text.decrease}><Minus size={15}/></button>
      <button type="button" className="ui-scale-value" onClick={reset} aria-label={text.reset} title={text.reset}><output>{formatUIScale(scale)}</output><RotateCcw size={12} aria-hidden="true"/></button>
      <button type="button" className="ui-scale-button" onClick={() => change(1)} disabled={scale >= UI_SCALE_MAX} aria-label={text.increase} title={text.increase}><Plus size={15}/></button>
    </div>
    <small className="ui-scale-hint">{text.hint}<span className="ui-scale-keyboard">{text.keyboard}</span></small>
  </div>
}

export default ScalePicker

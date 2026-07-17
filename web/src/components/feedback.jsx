import { Component, useEffect } from 'react'
import { X } from 'lucide-react'

// 统一的加载/错误反馈组件：路由级 Suspense fallback、行内 spinner、骨架屏、错误边界。
// 设计：沿用暖白磨砂 + 绿色强调色，尊重 prefers-reduced-motion。

export function Spinner({ label }) {
  return (
    <div className="ga-loading" role="status" aria-live="polite">
      <span className="ga-spinner" aria-hidden="true" />
      {label ? <span className="ga-loading-label">{label}</span> : null}
    </div>
  )
}

// 路由/页面级 Suspense 回退：居中显示，避免布局抖动。
export function RouteFallback({ label = '加载中…' }) {
  return (
    <div className="ga-route-fallback" role="status" aria-live="polite">
      <Spinner label={label} />
    </div>
  )
}

// 内容占位骨架屏：用于列表/卡片加载态。
export function Skeleton({ lines = 3, className = '' }) {
  return (
    <div className={`ga-skeleton ${className}`.trim()} aria-hidden="true">
      {Array.from({ length: lines }).map((_, i) => (
        <div key={i} className="ga-skeleton-line" />
      ))}
    </div>
  )
}

export const feedbackTone = message => {
  const text = String(message || '').toLowerCase()
  if (/失败|错误|无效|不可用|缺少|超时|拒绝|无法|必须|不能|不存在|failed|error|invalid|required|timeout|denied|cannot|unable|must be|does not|not found/.test(text)) return 'error'
  if (/正在|启动中|升级中|读取中|保存中|loading|starting|updating|saving/.test(text)) return 'progress'
  return 'success'
}

export function GlobalFeedback({ message, onDismiss, placement = 'bottom', successTimeout = 4500 }) {
  const tone = feedbackTone(message)
  useEffect(() => {
    if (!message || tone !== 'success' || !successTimeout) return undefined
    const timer = window.setTimeout(onDismiss, successTimeout)
    return () => window.clearTimeout(timer)
  }, [message, onDismiss, successTimeout, tone])
  if (!message) return null
  return <div className={`global-feedback global-feedback-${tone} is-${placement}`} role={tone === 'error' ? 'alert' : 'status'} aria-live={tone === 'error' ? 'assertive' : 'polite'}>
    <span className="global-feedback-mark" aria-hidden="true"/>
    <p>{message}</p>
    <button type="button" onClick={onDismiss} aria-label="关闭提示"><X size={16}/></button>
  </div>
}


export function ErrorFallback({ error, onReset, title = '页面加载失败' }) {
  const message = error?.message || String(error || 'Unknown error')
  return (
    <div className="ga-error-boundary" role="alert" aria-live="assertive">
      <div>
        <strong>{title}</strong>
        <p>当前页面模块渲染异常，其他导航仍可继续使用。</p>
        <code>{message}</code>
      </div>
      {onReset && <button type="button" onClick={onReset}>重试</button>}
    </div>
  )
}

export class ErrorBoundary extends Component {
  constructor(props) {
    super(props)
    this.state = { error: null }
  }

  static getDerivedStateFromError(error) {
    return { error }
  }

  componentDidCatch(error, info) {
    if (this.props.onError) this.props.onError(error, info)
  }

  componentDidUpdate(prevProps) {
    if (prevProps.resetKey !== this.props.resetKey && this.state.error) {
      this.setState({ error: null })
    }
  }

  reset = () => this.setState({ error: null })

  render() {
    if (this.state.error) {
      const Fallback = this.props.fallback || ErrorFallback
      return <Fallback error={this.state.error} onReset={this.reset} />
    }
    return this.props.children
  }
}

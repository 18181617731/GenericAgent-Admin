// 统一的加载反馈组件：路由级 Suspense fallback、行内 spinner、骨架屏。
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

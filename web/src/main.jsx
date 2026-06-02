import React, { Suspense, lazy } from 'react'
import { createRoot } from 'react-dom/client'
import './style.css'
import { RouteFallback } from './components/feedback.jsx'

// 按路由代码分割：/chat 仅加载 ChatApp，其余仅加载管理后台 App，
// 两个大入口不再被同一 bundle 静态打包。
const isChat = window.location.pathname.replace(/\/+$/, '') === '/chat'
const Root = lazy(() => (isChat ? import('./ChatApp.jsx') : import('./App.jsx')))

createRoot(document.getElementById('root')).render(
  <Suspense fallback={<RouteFallback label="正在加载界面…" />}>
    <Root />
  </Suspense>
)

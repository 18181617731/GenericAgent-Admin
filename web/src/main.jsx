import React, { Suspense, lazy } from 'react'
import { createRoot } from 'react-dom/client'
import { ConfigProvider } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import './style.css'
import { RouteFallback } from './components/feedback.jsx'

const isChat = window.location.pathname.replace(/\/+$/, '') === '/chat'
const Root = lazy(() => (isChat ? import('./ChatApp.jsx') : import('./App.jsx')))

createRoot(document.getElementById('root')).render(
  <ConfigProvider locale={zhCN} theme={{ token: { colorPrimary: '#10a37f', borderRadius: 10, fontFamily: 'Inter, system-ui, sans-serif' } }}>
    <Suspense fallback={<RouteFallback label="正在加载界面…" />}>
      <Root />
    </Suspense>
  </ConfigProvider>
)

import React, { Suspense, lazy, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { ConfigProvider, theme as antdTheme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import enUS from 'antd/locale/en_US'
import './style.css'
import { RouteFallback, ErrorBoundary } from './components/feedback.jsx'

const isChat = window.location.pathname.replace(/\/+$/, '') === '/chat'
const Root = lazy(() => (isChat ? import('./ChatApp.jsx') : import('./App.jsx')))

const storedLanguage = () => localStorage.getItem('ga-admin-lang-explicit') === '1' && localStorage.getItem('ga-admin-lang') === 'en' ? 'en' : 'zh'

function LocalizedRoot() {
  const [lang, setLang] = useState(storedLanguage)
  const [colorMode, setColorMode] = useState(() => document.documentElement.dataset.theme || localStorage.getItem('ga-admin-theme') || (window.matchMedia?.('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'))
  useEffect(() => {
    const onLanguageChange = event => setLang(event.detail === 'en' ? 'en' : 'zh')
    window.addEventListener('ga-admin-language-change', onLanguageChange)
    return () => window.removeEventListener('ga-admin-language-change', onLanguageChange)
  }, [])
  useEffect(() => {
    const onThemeChange = event => setColorMode(event.detail === 'dark' ? 'dark' : 'light')
    window.addEventListener('ga-admin-theme-change', onThemeChange)
    return () => window.removeEventListener('ga-admin-theme-change', onThemeChange)
  }, [])
  const loading = lang === 'en' ? 'Loading interface…' : '正在加载界面…'
  const dark = colorMode === 'dark'
  return <ConfigProvider locale={lang === 'en' ? enUS : zhCN} theme={{
    algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      colorPrimary: '#10a37f',
      borderRadius: 10,
      fontFamily: 'Inter, system-ui, sans-serif',
      colorBgBase: dark ? '#191c21' : '#ffffff',
      colorTextBase: dark ? '#ececf1' : '#171717',
      colorBorder: dark ? '#343a42' : 'rgba(23, 23, 23, .14)',
    },
  }}>
    <ErrorBoundary>
      <Suspense fallback={<RouteFallback label={loading} />}>
        <Root />
      </Suspense>
    </ErrorBoundary>
  </ConfigProvider>
}

createRoot(document.getElementById('root')).render(
  <LocalizedRoot />
)

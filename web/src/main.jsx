import React, { Suspense, lazy, useEffect, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { ConfigProvider, theme as antdTheme } from 'antd'
import zhCN from 'antd/locale/zh_CN'
import enUS from 'antd/locale/en_US'
import './style.css'
import { RouteFallback, ErrorBoundary } from './components/feedback.jsx'
import { applyThemeToDocument, getInitialTheme, getTheme, isThemeId } from './themes'
import { applyUIScaleToDocument, DEFAULT_UI_SCALE, getInitialUIScale, stepUIScale, UI_SCALE_STORAGE_KEY } from './lib/uiScale.js'
import { registerNotificationServiceWorker } from './lib/notifications.js'

// Chat is the primary interface: it owns "/" (and legacy "/chat").
// The admin console lives under "/admin" and acts as the settings area.
const rootPath = window.location.pathname.replace(/\/+$/, '')
const isAdmin = rootPath === '/admin' || rootPath.startsWith('/admin/')
// Each route imports on its own line because the build pairs one dependency
// list, stylesheets included, with one dynamic import expression. Choosing
// between two imports inside a single expression left both sharing the chat
// list, so the admin bundle's stylesheet was never linked in a build.
const AdminRoot = lazy(() => import('./App.jsx'))
const ChatRoot = lazy(() => import('./ChatApp.jsx'))
const Root = isAdmin ? AdminRoot : ChatRoot

void registerNotificationServiceWorker()

const storedLanguage = () => localStorage.getItem('ga-admin-lang-explicit') === '1' && localStorage.getItem('ga-admin-lang') === 'en' ? 'en' : 'zh'
const initialUIScale = getInitialUIScale()
applyUIScaleToDocument(initialUIScale)

function LocalizedRoot() {
  const [lang, setLang] = useState(storedLanguage)
  const [colorMode, setColorMode] = useState(getInitialTheme)
  const [uiScale, setUIScale] = useState(initialUIScale)
  useEffect(() => {
    const onLanguageChange = event => setLang(event.detail === 'en' ? 'en' : 'zh')
    window.addEventListener('ga-admin-language-change', onLanguageChange)
    return () => window.removeEventListener('ga-admin-language-change', onLanguageChange)
  }, [])
  useEffect(() => {
    const onThemeChange = event => setColorMode(current => isThemeId(event.detail) ? event.detail : current)
    window.addEventListener('ga-admin-theme-change', onThemeChange)
    return () => window.removeEventListener('ga-admin-theme-change', onThemeChange)
  }, [])
  useEffect(() => {
    const activeTheme = applyThemeToDocument(colorMode)
    localStorage.setItem('ga-admin-theme', activeTheme.id)
  }, [colorMode])
  useEffect(() => {
    document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en'
  }, [lang])
  useEffect(() => {
    const activeScale = applyUIScaleToDocument(uiScale)
    localStorage.setItem(UI_SCALE_STORAGE_KEY, String(activeScale))
    window.dispatchEvent(new CustomEvent('ga-admin-ui-scale-change', { detail: activeScale }))
  }, [uiScale])
  useEffect(() => {
    const onKeyDown = event => {
      if (!(event.ctrlKey || event.metaKey) || event.altKey) return
      if (!['+', '=', '-', '0'].includes(event.key)) return
      event.preventDefault()
      if (event.key === '0') setUIScale(DEFAULT_UI_SCALE)
      else setUIScale(current => stepUIScale(current, event.key === '-' ? -1 : 1))
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])
  const chooseLanguage = nextLanguage => {
    const activeLanguage = nextLanguage === 'en' ? 'en' : 'zh'
    localStorage.setItem('ga-admin-lang-explicit', '1')
    localStorage.setItem('ga-admin-lang', activeLanguage)
    setLang(activeLanguage)
  }
  const loading = lang === 'en' ? 'Loading interface…' : '正在加载界面…'
  const activeTheme = getTheme(colorMode)
  const algorithm = antdTheme[`${activeTheme.antdAlgorithm}Algorithm`] || antdTheme.defaultAlgorithm
  return <ConfigProvider locale={lang === 'en' ? enUS : zhCN} theme={{
    algorithm,
    token: {
      colorPrimary: '#10a37f',
      borderRadius: 10,
      fontFamily: 'Inter, system-ui, sans-serif',
      ...activeTheme.antdToken,
    },
  }}>
    <ErrorBoundary>
      <AuthGate lang={lang} theme={colorMode} onLanguageChange={chooseLanguage} onThemeChange={setColorMode}>
        <Suspense fallback={<RouteFallback label={loading} />}>
          <Root uiScale={uiScale} onUiScaleChange={setUIScale} />
        </Suspense>
      </AuthGate>
    </ErrorBoundary>
  </ConfigProvider>
}

createRoot(document.getElementById('root')).render(
  <LocalizedRoot />
)

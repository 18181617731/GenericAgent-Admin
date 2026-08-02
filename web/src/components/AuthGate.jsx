import React, { useEffect, useState } from 'react'
import ThemePicker from '../ThemePicker.jsx'

const copy = {
  zh: {
    checking: '正在检查管理员安全状态…',
    unavailable: '认证状态不可用',
    retry: '重试',
    kickerSetup: '首次运行安全',
    kickerChange: '管理员安全',
    titleSetup: '设置管理员密码',
    titleChange: '更改管理员密码',
    introSetup: '请先创建管理员密码，完成后才能访问设置和服务。',
    introChange: '访问任何设置或服务前，必须更新管理员凭据。',
    administrator: '管理员',
    currentPassword: '当前密码',
    newPassword: '新密码',
    confirmPassword: '确认新密码',
    saving: '正在更新…',
    submitSetup: '设置密码',
    submitChange: '设置新密码',
    note: '如果您从其他设备连接，更新后请使用新密码重新认证。',
    requestFailed: '认证检查失败',
    updateFailed: '密码更新失败',
    errors: {
      current_password_incorrect: '当前密码不正确。',
      password_confirmation_mismatch: '两次输入的密码不一致。',
      weak_password: '请使用至少 12 个字符。',
      managed_by_environment: '凭据由环境变量管理。',
      invalid_request: '请检查密码字段后重试。',
      setup_required: '请先设置管理员密码。',
    },
  },
  en: {
    checking: 'Checking administrator security…',
    unavailable: 'Authentication unavailable',
    retry: 'Try again',
    kickerSetup: 'FIRST-RUN SECURITY',
    kickerChange: 'ADMINISTRATOR SECURITY',
    titleSetup: 'Set the administrator password',
    titleChange: 'Change the administrator password',
    introSetup: 'Create an administrator password before accessing settings or services.',
    introChange: 'The administrator credential must be updated before any settings or services can be accessed.',
    administrator: 'Administrator',
    currentPassword: 'Current password',
    newPassword: 'New password',
    confirmPassword: 'Confirm new password',
    saving: 'Updating…',
    submitSetup: 'Set password',
    submitChange: 'Set new password',
    note: 'If you connected from another device, authenticate again with the new password after this update.',
    requestFailed: 'Authentication check failed',
    updateFailed: 'Password update failed',
    errors: {
      current_password_incorrect: 'Current password is incorrect.',
      password_confirmation_mismatch: 'The passwords do not match.',
      weak_password: 'Use at least 12 characters.',
      managed_by_environment: 'Credentials are managed by environment variables.',
      invalid_request: 'Check the password fields and try again.',
      setup_required: 'Set the administrator password first.',
    },
  },
}

async function readJSON(response) {
  try {
    return await response.json()
  } catch {
    return {}
  }
}

export function AuthGate({ children, lang = 'en', theme = 'warm', onThemeChange }) {
  const text = copy[lang] || copy.en
  const [state, setState] = useState({ loading: true, mustChange: false, initialized: true, username: 'admin', error: '' })
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [saving, setSaving] = useState(false)

  const loadStatus = async () => {
    setState(previous => ({ ...previous, loading: true, error: '' }))
    try {
      const response = await fetch('/api/auth/status', { headers: { Accept: 'application/json' }, cache: 'no-store' })
      const data = await readJSON(response)
      if (!response.ok) throw new Error(data.error || `${text.requestFailed} (${response.status})`)
      setState({
        loading: false,
        mustChange: Boolean(data.mustChangePassword),
        initialized: data.initialized !== false,
        username: data.username || 'admin',
        error: '',
      })
    } catch (error) {
      setState(previous => ({ ...previous, loading: false, error: error.message || `${text.requestFailed}.` }))
    }
  }

  useEffect(() => { loadStatus() }, [])

  const submit = async (event) => {
    event.preventDefault()
    if (newPassword !== confirmPassword) {
      setState(previous => ({ ...previous, error: text.errors.password_confirmation_mismatch }))
      return
    }
    setSaving(true)
    setState(previous => ({ ...previous, error: '' }))
    try {
      const response = await fetch('/api/auth/change-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ currentPassword: state.initialized ? currentPassword : '', newPassword, confirmPassword }),
      })
      const data = await readJSON(response)
      if (!response.ok) throw new Error(text.errors[data.error] || data.error || `${text.updateFailed} (${response.status})`)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setState(previous => ({ ...previous, initialized: true, mustChange: false, error: '' }))
    } catch (error) {
      setState(previous => ({ ...previous, error: error.message || `${text.updateFailed}.` }))
    } finally {
      setSaving(false)
    }
  }

  if (state.loading) return <div className="auth-gate-state" role="status">{text.checking}</div>
  if (!state.mustChange && !state.error) return children
  if (!state.mustChange) {
    return <main className="auth-gate"><section className="auth-card"><div className="auth-toolbar"><ThemePicker value={theme} onChange={onThemeChange} lang={lang} variant="compact" /></div><p className="auth-kicker">GA ADMIN</p><h1>{text.unavailable}</h1><p className="auth-error" role="alert">{state.error}</p><button type="button" onClick={loadStatus}>{text.retry}</button></section></main>
  }

  const firstSetup = !state.initialized
  return (
    <main className="auth-gate">
      <section className="auth-card" aria-labelledby="auth-title">
        <div className="auth-toolbar"><ThemePicker value={theme} onChange={onThemeChange} lang={lang} variant="compact" /></div>
        <p className="auth-kicker">{firstSetup ? text.kickerSetup : text.kickerChange}</p>
        <h1 id="auth-title">{firstSetup ? text.titleSetup : text.titleChange}</h1>
        <p className="auth-copy">{firstSetup ? text.introSetup : text.introChange}</p>
        <form onSubmit={submit}>
          <label>{text.administrator}<input value={state.username} readOnly autoComplete="username" /></label>
          {!firstSetup && <label>{text.currentPassword}<input type="password" value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} autoComplete="current-password" required autoFocus /></label>}
          <label>{text.newPassword}<input type="password" value={newPassword} onChange={event => setNewPassword(event.target.value)} autoComplete="new-password" minLength={12} required autoFocus={firstSetup} /></label>
          <label>{text.confirmPassword}<input type="password" value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} autoComplete="new-password" minLength={12} required /></label>
          {state.error && <p className="auth-error" role="alert">{state.error}</p>}
          <button type="submit" disabled={saving}>{saving ? text.saving : (firstSetup ? text.submitSetup : text.submitChange)}</button>
        </form>
        <p className="auth-note">{text.note}</p>
      </section>
    </main>
  )
}

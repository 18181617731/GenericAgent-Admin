import React, { useEffect, useState } from 'react'

const errorMessages = {
  current_password_incorrect: 'Current password is incorrect.',
  password_confirmation_mismatch: 'The passwords do not match.',
  weak_password: 'Use at least 12 characters and do not reuse the default password.',
  managed_by_environment: 'Credentials are managed by environment variables.',
  invalid_request: 'Check the password fields and try again.',
}

async function readJSON(response) {
  try {
    return await response.json()
  } catch {
    return {}
  }
}

export function AuthGate({ children }) {
  const [state, setState] = useState({ loading: true, mustChange: false, username: 'admin', error: '' })
  const [currentPassword, setCurrentPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [saving, setSaving] = useState(false)

  const loadStatus = async () => {
    setState(previous => ({ ...previous, loading: true, error: '' }))
    try {
      const response = await fetch('/api/auth/status', { headers: { Accept: 'application/json' }, cache: 'no-store' })
      const data = await readJSON(response)
      if (!response.ok) throw new Error(data.error || `Authentication check failed (${response.status})`)
      setState({ loading: false, mustChange: Boolean(data.mustChangePassword), username: data.username || 'admin', error: '' })
    } catch (error) {
      setState(previous => ({ ...previous, loading: false, error: error.message || 'Authentication check failed.' }))
    }
  }

  useEffect(() => { loadStatus() }, [])

  const submit = async (event) => {
    event.preventDefault()
    if (newPassword !== confirmPassword) {
      setState(previous => ({ ...previous, error: errorMessages.password_confirmation_mismatch }))
      return
    }
    setSaving(true)
    setState(previous => ({ ...previous, error: '' }))
    try {
      const response = await fetch('/api/auth/change-password', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Accept: 'application/json' },
        body: JSON.stringify({ currentPassword, newPassword, confirmPassword }),
      })
      const data = await readJSON(response)
      if (!response.ok) throw new Error(errorMessages[data.error] || data.error || `Password update failed (${response.status})`)
      setCurrentPassword('')
      setNewPassword('')
      setConfirmPassword('')
      setState(previous => ({ ...previous, mustChange: false, error: '' }))
    } catch (error) {
      setState(previous => ({ ...previous, error: error.message || 'Password update failed.' }))
    } finally {
      setSaving(false)
    }
  }

  if (state.loading) return <div className="auth-gate-state" role="status">Checking administrator security…</div>
  if (!state.mustChange && !state.error) return children
  if (!state.mustChange) {
    return <main className="auth-gate"><section className="auth-card"><p className="auth-kicker">GA ADMIN</p><h1>Authentication unavailable</h1><p className="auth-error" role="alert">{state.error}</p><button type="button" onClick={loadStatus}>Try again</button></section></main>
  }

  return (
    <main className="auth-gate">
      <section className="auth-card" aria-labelledby="auth-title">
        <p className="auth-kicker">FIRST-RUN SECURITY</p>
        <h1 id="auth-title">Change the default password</h1>
        <p className="auth-copy">The default administrator credential must be replaced before any settings or services can be accessed.</p>
        <form onSubmit={submit}>
          <label>Administrator<input value={state.username} readOnly autoComplete="username" /></label>
          <label>Current password<input type="password" value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} autoComplete="current-password" required autoFocus /></label>
          <label>New password<input type="password" value={newPassword} onChange={event => setNewPassword(event.target.value)} autoComplete="new-password" minLength={12} required /></label>
          <label>Confirm new password<input type="password" value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} autoComplete="new-password" minLength={12} required /></label>
          {state.error && <p className="auth-error" role="alert">{state.error}</p>}
          <button type="submit" disabled={saving}>{saving ? 'Updating…' : 'Set new password'}</button>
        </form>
        <p className="auth-note">If you connected from another device, authenticate again with the new password after this update.</p>
      </section>
    </main>
  )
}

import { useEffect, useRef, useState } from 'react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import { safeJson } from '../lib/format'
import {
  applyModelAndFailoverOrder,
  applyProviderOrder,
  mergePersistedModelOrder,
  normalizeFailoverGroups,
  orderedProviderProfiles,
} from '../lib/modelsEditor'

// mykey.py provider/model editor state. Everything funnels through
// persistProfiles so a single confirm gate covers every write to mykey.py.
export function useModelsConfig({ t, lang, setMsg, setBusy, active, onPersist }) {
  const [profiles, setProfiles] = useState([])
  const [persistedProfiles, setPersistedProfiles] = useState([])
  const [failoverGroups, setFailoverGroups] = useState([])
  const [preview, setPreview] = useState('')
  const [saveStatus, setSaveStatus] = useState({})
  const [importLoading, setImportLoading] = useState(false)
  const [revealedKeys, setRevealedKeys] = useState({})
  const [keyBusy, setKeyBusy] = useState({})
  const [instance, setInstance] = useState(null)
  const importAttempted = useRef(false)

  const instanceHeaders = () => (instance?.id ? { 'X-GA-Instance-ID': instance.id } : {})

  const getProfileKey = (idx, profile) => profile?.client_id
    || `${profile?.var_name || `profile_${idx + 1}`}:${profile?.type || 'native_oai'}:${profile?.apibase || ''}:${idx}`

  const importModels = async ({ quiet = false } = {}) => {
    if (!quiet) setBusy(true)
    setImportLoading(true)
    try {
      const d = await api('/api/models/import-mykey', { method:'POST', headers: instanceHeaders(), body: JSON.stringify({ reveal:false, save:false }) })
      const nextProfiles = orderedProviderProfiles(d.profiles || [])
      const nextGroups = normalizeFailoverGroups(d.failover_groups || [])
      setProfiles(nextProfiles)
      setPersistedProfiles(nextProfiles)
      setFailoverGroups(nextGroups)
      setSaveStatus({})
      setRevealedKeys({})
      setKeyBusy({})
      setPreview(safeJson(d))
      setMsg(`Loaded ${nextProfiles.length} profiles`)
    } catch (e) { setMsg(e.message) } finally { setImportLoading(false); if (!quiet) setBusy(false) }
  }

  useEffect(() => {
    if (!active || importAttempted.current || profiles.length) return
    importAttempted.current = true
    importModels({ quiet: true })
  }, [active, profiles.length])

  const previewModels = async () => {
    setBusy(true)
    try {
      const d = await api('/api/models/preview', { method:'POST', body: JSON.stringify({ profiles, failover_groups: failoverGroups }) })
      setPreview(d.python || safeJson(d))
    } catch (e) { setMsg(e.message) } finally { setBusy(false) }
  }

  const clearRevealedKey = (idx, profile) => {
    const key = getProfileKey(idx, profile || profiles[idx])
    setRevealedKeys(prev => { const next = { ...prev }; delete next[key]; delete next[idx]; return next })
    setKeyBusy(prev => { const next = { ...prev }; delete next[key]; delete next[idx]; return next })
  }

  const discoverModels = async ({ protocol, baseUrl, apiKey, varName } = {}) => {
    const params = new URLSearchParams()
    if (protocol) params.set('protocol', protocol)
    if (baseUrl) params.set('base_url', baseUrl)
    if (apiKey) params.set('api_key', apiKey)
    if (varName) params.set('var_name', varName)
    return api(`/api/models/discover?${params.toString()}`, { headers: instanceHeaders() })
  }

  const revealKey = async (idx, profile, refresh = false) => {
    const profileKey = getProfileKey(idx, profile || profiles[idx])
    if (!refresh && Object.prototype.hasOwnProperty.call(revealedKeys, profileKey)) {
      clearRevealedKey(idx, profile)
      return
    }
    setKeyBusy(prev => ({ ...prev, [profileKey]: true }))
    try {
      const d = await api('/api/models/raw', { dangerous: true, headers: instanceHeaders() })
      const rawProfiles = d?.profiles || []
      const varName = String(profile?.var_name || '').trim()
      const raw = (varName ? rawProfiles.find(p => String(p.var_name || '').trim() === varName) : null) || rawProfiles[idx]
      setRevealedKeys(prev => ({ ...prev, [profileKey]: String(raw?.apikey || profile?.apikey || '').trim() }))
    } catch (e) {
      setMsg(`Failed to reveal model key: ${e.message}`)
    } finally {
      setKeyBusy(prev => ({ ...prev, [profileKey]: false }))
    }
  }

  const persistProfiles = async (nextProfiles, { confirm = true, statusKeys = [], nextFailoverGroups = failoverGroups } = {}) => {
    if (confirm && !confirmDanger('models-save', lang === 'zh' ? '保存模型配置会更新 mykey.py，并可能覆盖当前启用配置。确认继续？' : 'Saving model configuration updates mykey.py and may overwrite the active configuration. Continue?')) return false
    if (statusKeys.length) setSaveStatus(current => ({ ...current, ...Object.fromEntries(statusKeys.map(k => [k, { status: 'saving', error: '', savedAt: null }])) }))
    setBusy(true)
    try {
      const cleanGroups = normalizeFailoverGroups(nextFailoverGroups)
      const d = await api('/api/models/export', { dangerous:true, method:'POST', headers: instanceHeaders(), body: JSON.stringify({ profiles: nextProfiles, failover_groups: cleanGroups, overwrite_active:true }) })
      const cleanProfiles = nextProfiles.map(({ previous_var_name: _previousVarName, ...profile }) => profile)
      setPersistedProfiles(cleanProfiles)
      setFailoverGroups(cleanGroups)
      setPreview(safeJson(d))
      if (statusKeys.length) {
        const savedAt = d.updated_at || new Date().toISOString()
        setSaveStatus(current => ({ ...current, ...Object.fromEntries(statusKeys.map(k => [k, { status: 'saved', error: '', savedAt }])) }))
      }
      setMsg(t.hints.modelsSaved)
      onPersist?.()
      return true
    } catch (e) {
      setMsg(e.message)
      if (statusKeys.length) setSaveStatus(current => ({ ...current, ...Object.fromEntries(statusKeys.map(k => [k, { status: 'error', error: e.message, savedAt: null }])) }))
      return false
    } finally { setBusy(false) }
  }

  const saveProfile = async (idx, profileKeyOverride, profileOverride) => {
    const profile = profileOverride || profiles[idx]
    if (!profile) return
    const profileKey = profileKeyOverride || getProfileKey(idx, profile)
    const nextPersisted = (persistedProfiles.length ? persistedProfiles : profiles).map(p => ({ ...p }))
    while (nextPersisted.length < profiles.length) nextPersisted.push({ ...profiles[nextPersisted.length] })
    const previousVarName = String(persistedProfiles[idx]?.var_name || '').trim()
    nextPersisted[idx] = previousVarName && previousVarName !== String(profile.var_name || '').trim()
      ? { ...profile, previous_var_name: previousVarName }
      : { ...profile }
    return await persistProfiles(nextPersisted, { confirm: false, statusKeys: [profileKey] })
  }

  const saveModelOrder = async (orderedRows) => {
    const { profiles: nextPersisted, failoverGroups: nextFailoverGroups } = applyModelAndFailoverOrder(persistedProfiles, failoverGroups, orderedRows)
    const ok = await persistProfiles(nextPersisted, { confirm: false, nextFailoverGroups })
    if (!ok) return false
    setProfiles(current => mergePersistedModelOrder(current, nextPersisted))
    setFailoverGroups(nextFailoverGroups)
    return true
  }

  const saveFailoverGroups = async (nextGroups) => persistProfiles(persistedProfiles, { confirm: false, nextFailoverGroups: nextGroups })

  const saveProviderOrder = async (orderedProfiles) => {
    const nextProfiles = applyProviderOrder(orderedProfiles)
    const ok = await persistProfiles(nextProfiles, { confirm: false })
    if (!ok) return false
    setProfiles(nextProfiles)
    return true
  }

  const addProfiles = async (newProfiles) => {
    const nextProfiles = [...profiles, ...newProfiles]
    const statusKeys = newProfiles.map((profile, i) => getProfileKey(profiles.length + i, profile))
    setProfiles(nextProfiles)
    const ok = await persistProfiles(nextProfiles, { confirm: false, statusKeys })
    if (!ok) setProfiles(profiles)
    return ok
  }

  const deleteProfile = async (nextProfiles) => {
    setProfiles(nextProfiles)
    const ok = await persistProfiles(nextProfiles, { confirm: false, statusKeys: [] })
    if (!ok) setProfiles(profiles)
    return ok
  }

  const patchProfile = (idx, patch) => {
    if (Object.prototype.hasOwnProperty.call(patch, 'apikey') || Object.prototype.hasOwnProperty.call(patch, 'var_name')) {
      clearRevealedKey(idx, profiles[idx])
    }
    setProfiles(ps => ps.map((p, i) => i === idx ? { ...p, ...patch } : p))
  }

  // Opening the editor for another GA instance must not inherit the previous
  // instance's drafts or revealed keys.
  const openFor = (nextInstance = null) => {
    setInstance(nextInstance)
    setProfiles([])
    setPersistedProfiles([])
    setFailoverGroups([])
    setPreview('')
    setSaveStatus({})
    setRevealedKeys({})
    importAttempted.current = false
  }

  return {
    profiles, setProfiles, persistedProfiles, failoverGroups, preview, saveStatus, importLoading,
    revealedKeys, keyBusy, instance,
    getProfileKey, importModels, previewModels, discoverModels, revealKey, clearRevealedKey,
    saveProfile, saveModelOrder, saveFailoverGroups, saveProviderOrder, addProfiles, deleteProfile, patchProfile,
    openFor,
  }
}

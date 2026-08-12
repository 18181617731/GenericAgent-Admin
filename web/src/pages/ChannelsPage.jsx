import React, { useEffect, useRef, useState } from 'react'
import { Globe2, RefreshCw, Save, Server } from 'lucide-react'
import { api } from '../lib/api'
import { confirmDanger } from '../lib/danger'
import { ChannelServiceTable, Panel, SecretInput } from '../components/common'

export function ChannelsPage({ frontendSvcs, t, actionStates = {}, onStart, onStop, onLogs, onAutostart, onReflectStart, onOpenHub }) {
  const [config, setConfig] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [testing, setTesting] = useState('')
  const [msg, setMsg] = useState(null)
  const [activeView, setActiveView] = useState('config')
  const tabRefs = useRef({})
  const text = t.channels
  const selectView = view => {
    setActiveView(view)
    tabRefs.current[view]?.focus()
  }
  const handleTabKeyDown = event => {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    if (event.key === 'Home') return selectView('config')
    if (event.key === 'End') return selectView('services')
    selectView(activeView === 'config' ? 'services' : 'config')
  }
  const profileName = profile => text.profileNames?.[profile.id] || profile.name
  const profileDescription = profile => text.profileDescriptions?.[profile.id] || profile.description
  const fieldLabel = field => text.fieldLabels?.[field.name] || field.label || field.name
  const fieldPlaceholder = field => field.name?.endsWith('_allowed_users') ? text.allowedUsersPlaceholder : (field.placeholder || '')
  const load = async () => {
    setLoading(true)
    try {
      const d = await api('/api/channels')
      setConfig(d)
      return d
    } catch (e) {
      setMsg({ kind: 'error', text: text.loadFailed(e.message) })
      return null
    } finally { setLoading(false) }
  }
  useEffect(() => { load() }, [])
  useEffect(() => { setMsg(null) }, [text])
  const patchField = (profileId, fieldName, value) => {
    setConfig(prev => ({
      ...(prev || {}),
      profiles: (prev?.profiles || []).map(p => p.id === profileId ? {
        ...p,
        fields: (p.fields || []).map(f => f.name === fieldName ? { ...f, value } : f)
      } : p)
    }))
  }
  const save = async () => {
    if (!confirmDanger('channels-save', text.saveConfirm)) return
    setSaving(true); setMsg({ kind: 'pending', text: text.saving })
    try {
      const d = await api('/api/channels', { dangerous:true, method:'PUT', body: JSON.stringify({ profiles: config?.profiles || [] }) })
      setConfig(d)
      setMsg({ kind: 'success', text: text.saved(d.path) })
    } catch (e) { setMsg({ kind: 'error', text: text.saveFailed(e.message) }) } finally { setSaving(false) }
  }
  const testProfile = async (profile) => {
    const name = profileName(profile)
    setTesting(profile.id); setMsg({ kind: 'pending', text: text.testing(name) })
    try {
      const d = await api('/api/channels/test', { method:'POST', body: JSON.stringify({ profile_id: profile.id, fields: profile.fields || [] }) })
      const detail = /[一-龥]/.test(d.message || '') && document.documentElement.lang === 'en' ? '' : (d.message || '')
      setMsg({ kind: d.ok ? 'success' : 'error', text: d.ok ? `${text.testPassed(name)}${detail ? ` · ${detail}` : ''}` : text.testFailed(name, detail) })
    } catch (e) { setMsg({ kind: 'error', text: text.testFailed(name, e.message) }) } finally { setTesting('') }
  }
  const runningCount = frontendSvcs.filter(s => s.running).length
  const profiles = config?.profiles || []
  const configuredCount = profiles.filter(profile => (profile.fields || []).some(field => field.has_value || String(field.value || '').trim())).length
  const channelTone = id => ({ feishu:'#3370ff', wecom:'#07c160', dingtalk:'#1677ff', discord:'#5865f2', qq:'#12b7f5', telegram:'#229ed9', wechat:'#2aae67' }[id] || '#6b7280')
  const channelMark = profile => ({ feishu:'飞', wecom:'企', dingtalk:'钉', discord:'D', qq:'Q', telegram:'T', wechat:'微' }[profile.id] || profileName(profile).slice(0, 1).toUpperCase())
  return <section className="channels-page">
    <div className="channel-console-metrics" aria-label="Channel overview">
      <div><span>{profiles.length || '—'}</span><small>{text.keyConfig}</small></div>
      <div><span>{configuredCount}</span><small>{text.savedField}</small></div>
      <div className={runningCount ? 'is-live' : ''}><span>{runningCount}<i/></span><small>{text.running} / {frontendSvcs.length}</small></div>
    </div>
    <div className="channel-view-tabs" role="tablist" aria-label={text.channelViews}>
      <button
        ref={node => { tabRefs.current.config = node }}
        id="channel-tab-config"
        className={activeView === 'config' ? 'is-active' : ''}
        role="tab"
        aria-selected={activeView === 'config'}
        aria-controls="channel-panel-config"
        tabIndex={activeView === 'config' ? 0 : -1}
        onClick={() => selectView('config')}
        onKeyDown={handleTabKeyDown}
      >
        <span className="channel-tab-icon"><Globe2 size={18}/></span>
        <span className="channel-tab-copy"><strong>{text.configTab}</strong><small>{text.configTabDesc}</small></span>
        <em>{configuredCount}/{profiles.length || 0}</em>
      </button>
      <button
        ref={node => { tabRefs.current.services = node }}
        id="channel-tab-services"
        className={activeView === 'services' ? 'is-active' : ''}
        role="tab"
        aria-selected={activeView === 'services'}
        aria-controls="channel-panel-services"
        tabIndex={activeView === 'services' ? 0 : -1}
        onClick={() => selectView('services')}
        onKeyDown={handleTabKeyDown}
      >
        <span className="channel-tab-icon"><Server size={18}/></span>
        <span className="channel-tab-copy"><strong>{text.servicesTab}</strong><small>{text.servicesTabDesc}</small></span>
        <em>{runningCount}/{frontendSvcs.length}</em>
      </button>
    </div>
    <div className="channels-layout">
      <section
        id="channel-panel-config"
        className="channels-panel channel-key-panel"
        role="tabpanel"
        aria-labelledby="channel-tab-config"
        hidden={activeView !== 'config'}
      >
        <div className="channel-workspace-head">
          <div>
            <span className="channel-section-index">01</span>
            <div><h3>{text.keyConfig}</h3><p>{profiles.length ? `${profiles.length} Channels` : text.loadingConfig}</p></div>
          </div>
          <div className="channel-workspace-actions">
            <button className="channel-icon-button" onClick={load} disabled={loading || saving} title={t.refresh} aria-label={t.refresh}><RefreshCw size={15} className={loading ? 'spin' : ''}/></button>
            <button className="primary channel-save-button" onClick={save} disabled={saving || loading || !config}><Save size={15}/>{saving ? t.busy : t.save}</button>
          </div>
        </div>
        <div className="channel-config-path">
          <span className="channel-path-light"/>
          <span>{config?.path ? text.configFile : (loading ? text.loadingConfig : text.noConfigPath)}</span>
          {config?.path && <code title={config.path}>{config.path}</code>}
        </div>
        {msg && <p className={`${msg.kind === 'error' ? 'err' : 'ok'} channel-message`}>{msg.text}</p>}
        <div className="channel-config-list">
          {profiles.map((profile, profileIndex) => {
            const completedFields = (profile.fields || []).filter(field => field.has_value || String(field.value || '').trim()).length
            return <article className="channel-config-card" key={profile.id} style={{ '--channel-tone': channelTone(profile.id) }}>
              <div className="channel-config-head">
                <div className="channel-identity">
                  <span className="channel-brand-mark">{channelMark(profile)}</span>
                  <div><div className="channel-title-line"><h3>{profileName(profile)}</h3><span>{String(profileIndex + 1).padStart(2, '0')}</span></div><p>{profileDescription(profile)}</p></div>
                </div>
                <div className="channel-card-actions">
                  <span className={`channel-field-count${completedFields ? ' has-config' : ''}`}>{completedFields}/{(profile.fields || []).length}</span>
                  {profile.testable && <button onClick={()=>testProfile(profile)} disabled={saving || testing === profile.id}>{testing === profile.id ? text.testingButton : text.testConnection}</button>}
                </div>
              </div>
              <div className="channel-fields">
                {(profile.fields || []).map(field => <label key={field.name}>
                  <span>{fieldLabel(field)}<small>{field.name}{field.secret && field.has_value ? ` · ${text.savedField}` : ''}</small></span>
                  {field.secret
                    ? <SecretInput value={field.value || ''} onChange={v=>patchField(profile.id, field.name, v)} t={t}/>
                    : field.type === 'bool'
                      ? <select value={String(field.value || 'false').toLowerCase()} onChange={e=>patchField(profile.id, field.name, e.target.value)}><option value="false">False</option><option value="true">True</option></select>
                      : <input value={field.value || ''} placeholder={fieldPlaceholder(field)} onChange={e=>patchField(profile.id, field.name, e.target.value)}/>}
                </label>)}
              </div>
            </article>
          })}
          {!loading && !profiles.length && <p className="empty-cell">{t.empty}</p>}
        </div>
      </section>
      <section
        id="channel-panel-services"
        className="channel-services-view"
        role="tabpanel"
        aria-labelledby="channel-tab-services"
        hidden={activeView !== 'services'}
      >
        <Panel title={t.lists.frontendServices} className="channels-panel channel-services-panel">
          <div className="channel-services-intro"><span className="channel-section-index">02</span><p>{t.desc.channels}</p></div>
          <ChannelServiceTable services={frontendSvcs} t={t} actionState={actionStates} onStart={onStart} onStop={onStop} onLogs={onLogs} onAutostart={onAutostart} onReflectStart={onReflectStart} onOpenHub={onOpenHub}/>
        </Panel>
      </section>
    </div>
  </section>
}

export default ChannelsPage

import React from 'react'
import { FolderCog, Globe2, Palette, Power, Save, ShieldAlert } from 'lucide-react'
import ThemePicker from '../ThemePicker'
import { SettingFooter, SettingNote, SettingRow, SettingToggle, SettingsPage, SettingsSection } from '../components/settings'

// Fields that belong to the config file this page edits. Everything else in the
// config object is owned by the backend and must survive a save untouched.
const CONFIG_FIELDS = ['ga_root', 'python_path', 'chat_data_dir', 'proxy_mode', 'http_proxy', 'https_proxy', 'all_proxy', 'no_proxy']

export const configDirty = (draft, saved) => {
  if (!draft || !saved) return false
  return CONFIG_FIELDS.some(field => String(draft[field] || '') !== String(saved[field] || ''))
}

const PROXY_MODES = ['off', 'system', 'custom']

export function GeneralPage({
  t, lang, text, cfg, setCfg, root, setRoot, savedCfg, onSave, busy,
  theme, setTheme, onLanguage, autostart, onToggleAutostart,
}) {
  const proxyMode = cfg?.proxy_mode || 'off'
  const dirty = configDirty({ ...(cfg || {}), ga_root: root }, savedCfg)
  const patch = (field, value) => setCfg({ ...(cfg || {}), [field]: value })
  const proxyHelp = { off: text.network.offHelp, system: text.network.systemHelp, custom: text.network.customHelp }[proxyMode]

  return <SettingsPage>
    <SettingsSection title={text.appearance.title} description={text.appearance.desc} icon={<Palette size={17}/>}>
      <SettingRow label={text.appearance.language} hint={text.appearance.languageHelp}>
        <div className="set-segmented" role="group" aria-label={t.language}>
          <button type="button" aria-pressed={lang === 'zh'} className={lang === 'zh' ? 'is-active' : ''} onClick={()=>onLanguage('zh')}>中文</button>
          <button type="button" aria-pressed={lang === 'en'} className={lang === 'en' ? 'is-active' : ''} onClick={()=>onLanguage('en')}>English</button>
        </div>
      </SettingRow>
      <SettingRow label={text.appearance.theme} hint={text.appearance.themeHelp}>
        <ThemePicker value={theme} onChange={setTheme} lang={lang}/>
      </SettingRow>
    </SettingsSection>

    <SettingsSection title={text.paths.title} description={text.paths.desc} icon={<FolderCog size={17}/>}>
      <SettingRow label={t.root} hint={text.paths.rootHelp} htmlFor="settings-ga-root" stacked>
        <input id="settings-ga-root" value={root} onChange={e=>setRoot(e.target.value)}/>
      </SettingRow>
      <SettingRow label={t.fields.pythonPath} hint={text.paths.pythonHelp} htmlFor="settings-python-path" stacked>
        <input id="settings-python-path" value={cfg?.python_path || ''} onChange={e=>patch('python_path', e.target.value)} placeholder={t.fields.pythonAuto}/>
      </SettingRow>
      <SettingRow label={t.fields.chatDataDir} hint={text.paths.dataHelp} htmlFor="settings-chat-data" stacked>
        <input id="settings-chat-data" value={cfg?.chat_data_dir || ''} onChange={e=>patch('chat_data_dir', e.target.value)} placeholder={t.fields.chatDataAuto}/>
      </SettingRow>
    </SettingsSection>

    <SettingsSection title={text.network.title} description={text.network.desc} icon={<Globe2 size={17}/>}>
      <SettingRow label={text.network.mode} hint={proxyHelp} htmlFor="settings-proxy-mode">
        <select id="settings-proxy-mode" value={proxyMode} onChange={e=>patch('proxy_mode', e.target.value)}>
          {PROXY_MODES.map(mode => <option key={mode} value={mode}>{text.network[mode]}</option>)}
        </select>
      </SettingRow>
      {proxyMode === 'custom' && <>
        <SettingRow label="HTTP_PROXY" htmlFor="settings-http-proxy">
          <input id="settings-http-proxy" value={cfg?.http_proxy || ''} onChange={e=>patch('http_proxy', e.target.value)} placeholder="http://127.0.0.1:7890"/>
        </SettingRow>
        <SettingRow label="HTTPS_PROXY" htmlFor="settings-https-proxy">
          <input id="settings-https-proxy" value={cfg?.https_proxy || ''} onChange={e=>patch('https_proxy', e.target.value)} placeholder="http://127.0.0.1:7890"/>
        </SettingRow>
        <SettingRow label="ALL_PROXY" htmlFor="settings-all-proxy">
          <input id="settings-all-proxy" value={cfg?.all_proxy || ''} onChange={e=>patch('all_proxy', e.target.value)} placeholder="socks5://127.0.0.1:7890"/>
        </SettingRow>
        <SettingRow label="NO_PROXY" htmlFor="settings-no-proxy">
          <input id="settings-no-proxy" value={cfg?.no_proxy || ''} onChange={e=>patch('no_proxy', e.target.value)} placeholder="localhost,127.0.0.1"/>
        </SettingRow>
      </>}
      <SettingFooter>
        <SettingNote tone="muted" icon={<ShieldAlert size={14}/>}>{text.confirmNote}</SettingNote>
        <span className={`set-dirty ${dirty ? 'is-dirty' : ''}`}>{dirty ? text.unsaved : text.saved}</span>
        <button className="primary" type="button" onClick={onSave} disabled={busy || !cfg || !dirty}><Save size={15}/>{busy ? t.busy : text.saveChanges}</button>
      </SettingFooter>
    </SettingsSection>

    <SettingsSection title={text.startup.title} description={text.startup.desc} icon={<Power size={17}/>}>
      <SettingToggle
        id="settings-autostart"
        checked={!!autostart?.enabled}
        disabled={busy || !autostart?.supported}
        onChange={onToggleAutostart}
        label={t.autostart}
        hint={!autostart?.supported ? t.hints.autostartUnsupported : text.startup.autostartHelp}
        onText={t.enabled}
        offText={autostart?.supported ? t.disabled : t.unsupported}
      />
      {autostart?.path && <p className="set-path"><code>{autostart.path}</code></p>}
    </SettingsSection>
  </SettingsPage>
}

export default GeneralPage

import { Eye, EyeOff, RefreshCw, UploadCloud } from 'lucide-react'
import { emptyProfile } from '../lib/format'
import { Panel, SecretInput } from '../components/common'

export function Models({ t, profiles, setProfiles, patchProfile, importModels, previewModels, saveModels, modelPreview, revealModels, revealedKeys = {} }) {
  const hasRevealed = Object.keys(revealedKeys).length > 0

  return (
    <section>
      <div className="model-top">
        <div>
          <h3>{t.nav.models}</h3>
          <p>{t.hints.previewHelp}</p>
        </div>
        <div className="actions">
          <button onClick={importModels}><RefreshCw size={14}/>{t.hints.modelSource}</button>
          <button onClick={() => setProfiles([...profiles, emptyProfile(profiles.length)])}>{t.hints.addProfile}</button>
          <button onClick={previewModels}><Eye size={14}/>{t.hints.preview}</button>
          <button onClick={revealModels} title={t.hints.revealKeys || 'Reveal API Keys'}>
            {hasRevealed ? <EyeOff size={14}/> : <Eye size={14}/>}
            {t.hints.revealKeys || 'Reveal Keys'}
          </button>
          <button onClick={saveModels}><UploadCloud size={14}/>{t.hints.writeMykey}</button>
        </div>
      </div>

      <div className="models-layout">
        <div className="profiles">
          {profiles.map((p, idx) => (
            <div className="profile" key={idx}>
              <div className="profile-head">
                <b>#{idx + 1} {p.name || p.var_name}</b>
                <label>
                  <input type="checkbox" checked={!!p.enabled} onChange={(e) => patchProfile(idx, { enabled: e.target.checked })}/>
                  {' '}enabled
                </label>
              </div>
              <div className="form-grid">
                <label>{t.fields.varName}<input value={p.var_name || ''} onChange={(e) => patchProfile(idx, { var_name: e.target.value })}/></label>
                <label>{t.fields.type}<input value={p.type || ''} onChange={(e) => patchProfile(idx, { type: e.target.value })}/></label>
                <label>{t.fields.name}<input value={p.name || ''} onChange={(e) => patchProfile(idx, { name: e.target.value })}/></label>
                <label>{t.fields.model}<input value={p.model || ''} onChange={(e) => patchProfile(idx, { model: e.target.value })}/></label>
                <label className="span2">{t.fields.apiBase}<input value={p.apibase || ''} onChange={(e) => patchProfile(idx, { apibase: e.target.value })}/></label>
                <label className="span2">
                  {t.fields.apiKey}
                  {revealedKeys[p.var_name]
                    ? <input
                        type="text"
                        readOnly
                        value={revealedKeys[p.var_name]}
                        className="secret-revealed"
                        style={{ fontFamily: 'monospace', background: 'var(--bg2,#f5f5f5)' }}
                      />
                    : <SecretInput value={p.apikey} onChange={(v) => patchProfile(idx, { apikey: v })} t={t}/>
                  }
                </label>
                <label>{t.fields.stream}
                  <select value={String(!!p.stream)} onChange={(e) => patchProfile(idx, { stream: e.target.value === 'true' })}>
                    <option value="true">true</option>
                    <option value="false">false</option>
                  </select>
                </label>
                <label>{t.fields.maxRetries}<input type="number" value={p.max_retries ?? 3} onChange={(e) => patchProfile(idx, { max_retries: Number(e.target.value) })}/></label>
                <label>{t.fields.readTimeout}<input type="number" value={p.read_timeout ?? 300} onChange={(e) => patchProfile(idx, { read_timeout: Number(e.target.value) })}/></label>
                <label>{t.fields.reasoningEffort}<input value={p.reasoning_effort || ''} onChange={(e) => patchProfile(idx, { reasoning_effort: e.target.value })}/></label>
              </div>
            </div>
          ))}
        </div>
        <Panel title={t.lists.generatedPreview} className="preview">
          <pre>{modelPreview || t.empty}</pre>
        </Panel>
      </div>
    </section>
  )
}

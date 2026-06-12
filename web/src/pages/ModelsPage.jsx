import { AlertTriangle, CheckCircle2, Eye, RefreshCw, UploadCloud } from 'lucide-react'
import { emptyProfile } from '../lib/format'
import { modelRiskCatalog, modelValidationSummary, validateModelProfiles } from '../lib/modelsValidation'
import { Panel, SecretInput } from '../components/common'

const VALIDATION_COPY = {
  valid: '校验通过',
  blocked: '写入前请先修复阻断项',
  warnings: '警告',
  errors: '阻断项',
  saveDisabled: '写入 mykey.py 前请先修复红色校验项。',
  keys: {
    varNameRequired: '必须填写变量名',
    varNameInvalid: '变量名必须是 Python 标识符',
    varNameDiscoveryToken: '变量名必须包含 api、config 或 cookie，便于 GA 发现',
    varNameDuplicate: '变量名重复',
    nameRequired: '必须填写名称',
    modelRequired: '必须填写模型',
    apiBaseRequired: '必须填写 API Base',
    apiBaseProtocol: 'API Base 应以 http:// 或 https:// 开头',
    maxRetriesInvalid: '重试次数必须大于或等于 0',
    readTimeoutInvalid: '超时必须大于 0',
    apiKeyEmpty: 'API Key 为空；仅用于本地或无认证端点'
  }
}

function ValidationList({ title, items, copy, tone }) {
  if (!items?.length) return null
  return <div className={`validation-list ${tone}`}><span>{title}</span><ul>{items.map((key) => <li key={key}>{copy.keys[key] || key}</li>)}</ul></div>
}

export function Models({ t, profiles, setProfiles, patchProfile, importModels, previewModels, saveModels, modelPreview, riskCatalog, riskCatalogError }) {
  const validation = validateModelProfiles(profiles)
  const summary = modelValidationSummary(validation)
  const copy = VALIDATION_COPY
  const hasErrors = summary.errors > 0
  const hasProfiles = profiles.length > 0

  return <section>
    <div className="model-top">
      <div><h3>{t.nav.models}</h3><p>{t.hints.previewHelp}</p></div>
      <div className="actions">
        <button onClick={importModels}><RefreshCw size={14}/>{t.hints.modelSource}</button>
        <button onClick={() => setProfiles([...profiles, emptyProfile(profiles.length)])}>{t.hints.addProfile}</button>
        <button onClick={previewModels}><Eye size={14}/>{t.hints.preview}</button>
        <button onClick={saveModels} disabled={hasErrors} title={hasErrors ? copy.saveDisabled : t.hints.writeMykey}><UploadCloud size={14}/>{t.hints.writeMykey}</button>
      </div>
    </div>
    <div className={`model-validation-summary ${hasErrors ? 'has-errors' : 'ok'}`} role={hasErrors ? 'alert' : 'status'}>
      {hasErrors ? <AlertTriangle size={16}/> : <CheckCircle2 size={16}/>}
      <b>{hasErrors ? copy.blocked : copy.valid}</b>
      <span>{copy.errors}: {summary.errors}</span>
      <span>{copy.warnings}: {summary.warnings}</span>
    </div>
    <p className="operation-note" role="note">导入和预览仅执行只读检查。写回 mykey.py 会修改本地配置，并可能替换生成的模型条目；请先审核预览并修复红色校验项。</p>
    <div className="models-layout">
      <div className="profiles">{!hasProfiles && <div className="empty-card" role="status"><b>尚未加载模型 Profile</b><span>如果 mykey.py 已存在，可先导入；或新增 Profile，在预览前填写名称、模型、API Base 和 Key。</span></div>}{profiles.map((p, idx) => {
        const item = validation[idx] || { errors: [], warnings: [], ok: true }
        return <div className={`profile ${item.errors.length ? 'has-errors' : item.warnings.length ? 'has-warnings' : ''}`} key={idx}>
          <div className="profile-head">
            <b>#{idx + 1} {p.name || p.var_name || t.nav.models}</b>
            <span className={`profile-status ${item.errors.length ? 'bad' : item.warnings.length ? 'warn' : 'ok'}`}>{item.errors.length ? `${copy.errors}: ${item.errors.length}` : item.warnings.length ? `${copy.warnings}: ${item.warnings.length}` : copy.valid}</span>
          </div>
          <ValidationList title={copy.errors} items={item.errors} copy={copy} tone="error"/>
          <ValidationList title={copy.warnings} items={item.warnings} copy={copy} tone="warning"/>
          <div className="form-grid">
            <label>{t.fields.varName}<input value={p.var_name || ''} onChange={(e) => patchProfile(idx, { var_name: e.target.value })}/></label>
            <label>{t.fields.type}<input value={p.type || ''} onChange={(e) => patchProfile(idx, { type: e.target.value })}/></label>
            <label>{t.fields.name}<input value={p.name || ''} onChange={(e) => patchProfile(idx, { name: e.target.value })}/></label>
            <label>{t.fields.model}<input value={p.model || ''} onChange={(e) => patchProfile(idx, { model: e.target.value })}/></label>
            <label className="span2">{t.fields.apiBase}<input value={p.apibase || ''} onChange={(e) => patchProfile(idx, { apibase: e.target.value })}/></label>
            <label className="span2">{t.fields.apiKey}<SecretInput value={p.apikey} onChange={(v) => patchProfile(idx, { apikey: v })} t={t}/></label>
            <label>{t.fields.stream}<select value={String(!!p.stream)} onChange={(e) => patchProfile(idx, { stream: e.target.value === 'true' })}><option value="true">true</option><option value="false">false</option></select></label>
            <label>{t.fields.maxRetries}<input type="number" value={p.max_retries ?? 3} onChange={(e) => patchProfile(idx, { max_retries: Number(e.target.value) })}/></label>
            <label>{t.fields.readTimeout}<input type="number" value={p.read_timeout ?? 300} onChange={(e) => patchProfile(idx, { read_timeout: Number(e.target.value) })}/></label>
            <label>{t.fields.reasoningEffort}<input value={p.reasoning_effort || ''} onChange={(e) => patchProfile(idx, { reasoning_effort: e.target.value })}/></label>
          </div>
        </div>
      })}</div>
      <Panel title="模型路由安全" className="model-risk-panel">
        <div className={`model-risk-summary ${risk.status}`}>
          <AlertTriangle size={16}/>
          <div><b>{risk.status === 'ready' ? '风险目录已加载' : risk.status === 'error' ? '风险目录不可用' : '风险目录没有 /api/models 条目'}</b><p>{risk.status === 'error' ? risk.error : '保存/导出会写入 GA 模型配置，并继续受 confirmDanger 门禁保护；不显示密钥的预览/导入保持只读。'}</p></div>
        </div>
        <div className="model-risk-grid">
          {risk.items.map(item => <div className="model-risk-item" key={`${item.method}-${item.route}-${item.action}`}>
            <span className={`badge ${item.level}`}>{item.level || '待审核'}</span><b>{item.method} {item.route}</b><small>{item.action || item.reason || '目录条目'}</small>
          </div>)}
          {risk.items.length === 0 && <p className="muted">当前没有实时模型路由条目；不要因为目录为空就推断为安全。</p>}
        </div>
        {risk.missingConfirmedWriteRoutes.length > 0 && <p className="err-text">目录中缺少已确认写入门禁：{risk.missingConfirmedWriteRoutes.join(', ')}</p>}
      </Panel>
      <Panel title={t.lists.generatedPreview} className="preview"><pre>{modelPreview || (hasProfiles ? t.empty : '添加至少一个模型配置后将显示预览。')}</pre></Panel>
    </div>
  </section>
}

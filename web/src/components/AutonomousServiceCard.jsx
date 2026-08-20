import React from 'react'
import { ChevronDown, CircleHelp, Eye, Play, Square } from 'lucide-react'
import { ProviderModelCascade, buildModelProviderGroups, findModelProviderValue, modelProvider, runtimeModelLabel } from './ModelProviderCascade.jsx'
import { autonomousServiceView } from '../lib/autonomous.js'
import { firstRuntimeModel, runtimeModelDescription } from '../lib/modelDefaults.js'

const serviceCommand = service => Array.isArray(service?.command) ? service.command.join(' ') : (service?.command || '-')

export function AutonomousServiceCard({ service, lang = 'zh', llms = [], actionState, onStart, onStop, onLogs, onAutostart, onModel, showModel = true, compact = false }) {
  const zh = lang !== 'en'
  const view = autonomousServiceView(service, lang)
  const pending = actionState?.status === 'pending'
  const firstModel = firstRuntimeModel(llms)
  const defaultLabel = firstModel
    ? `${zh ? '默认模型' : 'Default model'}: ${runtimeModelDescription(firstModel)}`
    : (zh ? '默认模型' : 'Default model')
  const modelGroups = buildModelProviderGroups(llms, { defaultLabel, defaultProviderLabel: zh ? '默认模型' : 'Default model', defaultModel: firstModel })
  const modelValue = service?.model_no ?? ''
  const selectedProvider = findModelProviderValue(modelGroups, modelValue)
  const modelMatch = modelValue === '' ? null : llms.find(model => Number(model.index) === Number(modelValue))
  const modelText = modelMatch ? `${modelProvider(modelMatch)} · ${runtimeModelLabel(modelMatch)}` : (zh ? '默认模型' : 'Default model')
  const resolvedModelText = modelMatch ? modelText : defaultLabel
  const retry = actionState?.action === 'stop' ? onStop : onStart

  return <article className={`autonomous-service ${compact ? 'is-compact' : ''} ${service?.running ? 'is-running' : 'is-stopped'}`} aria-busy={pending || undefined}>
    <div className="autonomous-service-head">
      <div className="autonomous-service-title">
        <b title={compact ? `${view.description}（${view.technicalName}）` : undefined}>{view.title}</b>
        {!compact && <span>{view.description}</span>}
        {!compact && <code title={view.technicalName} className="autonomous-service-technical">{view.technicalName}</code>}
      </div>
      <div className="autonomous-service-state">
        {!compact && <span className="autonomous-help" tabIndex="0" title={view.help} aria-label={`${view.title}：${view.help}`} data-tooltip={view.help}><CircleHelp size={16}/></span>}
        <span className={service?.running ? 'status-pill running' : 'status-pill stopped'}>{service?.running ? (zh ? '运行中' : 'Running') : (zh ? '已停止' : 'Stopped')}</span>
      </div>
    </div>

    <div className="autonomous-service-controls">
      {showModel && <div className="autonomous-model-control">
        <span>{zh ? '执行模型' : 'Execution model'}</span>
        {service?.running
          ? <b title={resolvedModelText}>{resolvedModelText}</b>
          : <ProviderModelCascade groups={modelGroups} selectedProvider={selectedProvider} value={modelValue} showLabel={false} placement="auto" align="start" className="service-provider-cascade" onChange={value => onModel?.(service.name, value === '' ? null : Number(value))}/>}
      </div>}
      <label className="autonomous-autostart"><input type="checkbox" checked={!!service?.autostart} onChange={event => onAutostart?.(service.name, event.target.checked)}/><span>{zh ? '随 GA Admin 启动' : 'Start with GA Admin'}</span></label>
      <div className="autonomous-service-actions">
        <button type="button" onClick={() => onStart?.(service.name)} disabled={pending || service?.running}><Play size={15}/>{zh ? '启动' : 'Start'}</button>
        <button type="button" onClick={() => onStop?.(service.name)} disabled={pending || !service?.running}><Square size={14}/>{zh ? '停止' : 'Stop'}</button>
        <button type="button" className="secondary" onClick={() => onLogs?.(service.name)}><Eye size={15}/>{zh ? '日志' : 'Logs'}</button>
      </div>
    </div>

    {compact && <details className="autonomous-runtime-details"><summary><ChevronDown size={14}/>{zh ? '说明与运行详情' : 'About & runtime'}{service?.pid ? ` · PID ${service.pid}` : ''}</summary><p className="autonomous-service-compact-intro">{view.description}（<code>{view.technicalName}</code>）</p><p className="autonomous-service-compact-help">{view.help}</p></details>}
    {!compact && <details className="autonomous-runtime-details">
      <summary><ChevronDown size={14}/>{zh ? '运行详情' : 'Runtime details'}{service?.pid ? ` · PID ${service.pid}` : ''}</summary>
      <dl>
        <div><dt>{zh ? '启动时间' : 'Started'}</dt><dd>{service?.started_at || '-'}</dd></div>
        <div><dt>{zh ? '返回码' : 'Exit code'}</dt><dd>{service?.returncode ?? service?.return_code ?? '-'}</dd></div>
        <div><dt>{zh ? '工作目录' : 'Working directory'}</dt><dd title={service?.workdir}>{service?.workdir || '-'}</dd></div>
        <div><dt>{zh ? '命令' : 'Command'}</dt><dd title={serviceCommand(service)}>{serviceCommand(service)}</dd></div>
      </dl>
    </details>}
    {actionState?.message && <div className={`service-action-status ${actionState.status || ''}`} role={actionState.status === 'error' ? 'alert' : 'status'} aria-live="polite"><span>{actionState.message}</span>{actionState.status === 'error' && <button type="button" onClick={() => retry?.(service.name)}>{zh ? '重试' : 'Retry'}</button>}</div>}
  </article>
}

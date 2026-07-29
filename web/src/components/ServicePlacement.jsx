import React from 'react'
import { ListChecks, ShieldCheck, Users } from 'lucide-react'
import { autonomousServiceView } from '../lib/autonomous.js'
import { AutonomousServiceCard } from './AutonomousServiceCard.jsx'

const WORKFLOW_COMPONENTS = [
  {
    name: 'reflect/agent_team_worker.py',
    icon: Users,
    zh: ['Hive 团队执行', '需要本次任务的 BBS 地址、密钥和 worker 名称，由 Goal Hive 按需启动并回收。'],
    en: ['Hive team execution', 'Requires the run-specific BBS URL, key, and worker name. Goal Hive starts and retires it as needed.'],
  },
  {
    name: 'reflect/checklist_master.py',
    icon: ListChecks,
    zh: ['Checklist / MapReduce 编排', '需要本次任务的状态目录，由 Checklist 工作流创建并持续推进清单。'],
    en: ['Checklist / MapReduce orchestration', 'Requires the run-specific state directory and is created by the Checklist workflow to advance its task list.'],
  },
]

export function EnvironmentGuardianSection({ services = [], lang = 'zh', actionStates = {}, onStart, onStop, onLogs, onAutostart }) {
  const zh = lang !== 'en'
  if (!services.length) return null
  return <section className="environment-guardian" aria-label={zh ? '运行保障' : 'Runtime protection'}>
    <header><div><ShieldCheck size={19}/><b>{zh ? '运行保障' : 'Runtime protection'}</b></div><p>{zh ? '服务看护器只检查 scheduler 和 autonomous 的端口并尝试恢复，不调用模型，也不能替代完整的系统健康检查。' : 'The watchdog only checks scheduler and autonomous ports and attempts recovery. It does not call a model or replace the full system health check.'}</p></header>
    <div className="environment-guardian-list">{services.map(service => <AutonomousServiceCard key={service.name} service={service} lang={lang} actionState={actionStates[service.name]} onStart={onStart} onStop={onStop} onLogs={onLogs} onAutostart={onAutostart} showModel={false}/>)}</div>
  </section>
}

export function GoalWorkflowGuide({ services = [], lang = 'zh' }) {
  const zh = lang !== 'en'
  return <section className="goal-workflow-guide" aria-label={zh ? 'Goal 协作组件' : 'Goal collaboration components'}>
    <header><b>{zh ? 'Goal 协作组件' : 'Goal collaboration components'}</b><span>{zh ? '以下组件属于单次任务运行，不是需要常驻的后台服务。' : 'These components belong to an individual run and are not persistent background services.'}</span></header>
    <div className="goal-workflow-list">{WORKFLOW_COMPONENTS.map(item => {
      const service = services.find(candidate => candidate.name === item.name)
      const view = autonomousServiceView(service || { name: item.name }, lang)
      const copy = zh ? item.zh : item.en
      const Icon = item.icon
      return <div className="goal-workflow-row" key={item.name}>
        <Icon size={18}/><div><b>{view.title}</b><span>{copy[0]}</span><p>{copy[1]}</p><code>{item.name}</code></div><em className={service ? 'is-ready' : 'is-missing'}>{service ? (zh ? '脚本可用' : 'Available') : (zh ? '脚本缺失' : 'Missing')}</em>
      </div>
    })}</div>
    <p className="goal-workflow-note">{zh ? '它们会唤醒 Agent 并产生模型调用，但启动参数必须绑定具体任务；因此不提供独立启动、开机自启或全局模型配置。Hive worker 使用本次 Goal 选择的模型。' : 'They wake an Agent and can consume model calls, but their launch parameters must be bound to a specific run. They therefore have no standalone start, autostart, or global model setting. Hive workers use the model selected for the Goal run.'}</p>
  </section>
}

import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { buildScheduleCreateRequest, effectiveScheduleModelNo, firstScheduleTaskID, normalizeScheduleLatestRun, normalizeScheduleModelNo, normalizeScheduleTasksPayload } from './schedule.js'

test('normalizeScheduleTasksPayload gives stable empty and row states', () => {
  assert.deepEqual(normalizeScheduleTasksPayload(null).tasks, [])
  const state = normalizeScheduleTasksPayload({ tasks: [{ id: 'daily', enabled: true }, null] })
  assert.equal(state.tasks.length, 1)
  assert.equal(state.tasks[0].status, 'enabled')
  assert.deepEqual(state.tasks[0].recent_reports, [])
  assert.equal(state.tasks[0].latest_run.status, 'never_run')
})

test('normalizeScheduleLatestRun preserves execution result independently of enabled config', () => {
  const failed = normalizeScheduleTasksPayload({ tasks: [{ id: 'daily', enabled: false, latest_run: { status: 'ERROR', executed_at: '2026-08-29T10:00:00Z', reason: 'login timed out', report_path: 'sche_tasks/done/run.md' } }] }).tasks[0]
  assert.equal(failed.enabled, false)
  assert.equal(failed.status, 'disabled')
  assert.deepEqual(failed.latest_run, {
    status: 'failed', executed_at: '2026-08-29T10:00:00Z', summary: '', reason: 'login timed out', report_path: 'sche_tasks/done/run.md',
  })
  assert.equal(normalizeScheduleLatestRun({ status: 'PARTIAL' }).status, 'partial')
  assert.equal(normalizeScheduleLatestRun({ status: 'pending' }).status, 'waiting')
  assert.equal(normalizeScheduleLatestRun({ status: 'SKIPPED' }).status, 'skipped')
  assert.equal(normalizeScheduleLatestRun({ status: 'unexpected' }).status, 'unknown')
})

test('normalizeScheduleLatestRun falls back to latest report metadata and never-run', () => {
  assert.deepEqual(normalizeScheduleLatestRun(null, { mod_time: '2026-08-29T09:00:00Z', path: 'sche_tasks/done/legacy.md' }), {
    status: 'unknown', executed_at: '2026-08-29T09:00:00Z', summary: '', reason: '', report_path: 'sche_tasks/done/legacy.md',
  })
  assert.equal(normalizeScheduleLatestRun(null, null).status, 'never_run')
})

test('buildScheduleCreateRequest trims id and includes default task body', () => {
  const req = buildScheduleCreateRequest(' demo ', { prompt: 'hello' })
  assert.deepEqual(req, { id: 'demo', task: { schedule: '09:00', repeat: 'daily', enabled: false, prompt: 'hello' } })
})

test('firstScheduleTaskID selects the first valid task card', () => {
  assert.equal(firstScheduleTaskID([{ id: 'first' }, { id: 'second' }]), 'first')
  assert.equal(firstScheduleTaskID([null, { name: 'fallback' }]), 'fallback')
  assert.equal(firstScheduleTaskID([]), '')
})

test('effectiveScheduleModelNo resolves task overrides and scheduler fallback', () => {
  assert.equal(effectiveScheduleModelNo({ llm_no: 4 }, 17), 4)
  assert.equal(effectiveScheduleModelNo({ llm_no: null }, 17), 17)
  assert.equal(effectiveScheduleModelNo({}, 'invalid'), 0)
  assert.equal(normalizeScheduleModelNo('', 19), 19)
})

test('schedule UI refreshes /api/schedule/tasks and confirms dangerous create', () => {
  const app = readFileSync(new URL('../App.jsx', import.meta.url), 'utf8')
  const workbench = readFileSync(new URL('../components/ScheduledTaskWorkbench.jsx', import.meta.url), 'utf8')
  assert.match(app, /api\('\/api\/schedule\/tasks'\)/)
  assert.match(app, /const loadScheduleTasks = async/)
  assert.match(app, /setScheduleError\(e\.message\)/)
  assert.match(app, /window\.setInterval\(poll, 15000\)/)
  assert.match(app, /setScheduleData\(normalizedSchedule\)/)
  assert.match(app, /notificationMonitorRef\.current\.busy/)
  assert.match(app, /return \(\) => \{ active = false; window\.clearInterval\(timer\) \}/)
  assert.match(app, /confirmDanger\('schedule-create'/)
  assert.match(app, /api\('\/api\/schedule\/create', \{ dangerous:true, method:'POST'/)
  assert.ok(app.includes("api('/api/schedule/run'"))
  assert.ok(app.includes('/api/schedule/run/status?run_id='))
  assert.match(workbench, /<RefreshCw size=\{15\}\/>/)
  assert.match(workbench, /copy\.noMatch/)
})


test('normalizeScheduleTasksPayload fills missing task identity and disabled state', () => {
  const normalized = normalizeScheduleTasksPayload({ tasks: [{ name: '', enabled: false }, { name: 'nightly', enabled: true, recent_reports: null }] })
  assert.equal(normalized.version, 'unknown')
  assert.equal(normalized.tasks[0].id, 'task-1')
  assert.equal(normalized.tasks[0].status, 'disabled')
  assert.equal(normalized.tasks[0].schedule, 'unscheduled')
  assert.equal(normalized.tasks[0].repeat, 'manual')
  assert.deepEqual(normalized.tasks[1].recent_reports, [])
  assert.equal(normalized.tasks[1].status, 'enabled')
})

test('normalizeScheduleTasksPayload preserves error states without stale enabled success', () => {
  const state = normalizeScheduleTasksPayload({ enabled: true, version: '', error: 'schedule endpoint failed', tasks: 'stale' })
  assert.equal(state.enabled, false)
  assert.equal(state.error, 'schedule endpoint failed')
  assert.equal(state.version, 'unknown')
  assert.deepEqual(state.tasks, [])
})

test('normalizeScheduleTasksPayload handles null and missing task fields as disabled rows', () => {
  const state = normalizeScheduleTasksPayload({ enabled: false, tasks: [{ id: 0, name: '', status: '', schedule: '', repeat: '', prompt: null, recent_reports: 'stale' }] })
  assert.equal(state.tasks.length, 1)
  assert.equal(state.tasks[0].id, 'task-1')
  assert.equal(state.tasks[0].enabled, false)
  assert.equal(state.tasks[0].status, 'disabled')
  assert.equal(state.tasks[0].schedule, 'unscheduled')
  assert.equal(state.tasks[0].repeat, 'manual')
  assert.equal(state.tasks[0].prompt, '')
  assert.deepEqual(state.tasks[0].recent_reports, [])
})

test('normalizeScheduleTasksPayload preserves a valid task model selection', () => {
  const state = normalizeScheduleTasksPayload({ tasks: [{ id: 'daily', enabled: true, llm_no: 7 }] })
  assert.equal(state.tasks[0].llm_no, 7)
  assert.equal(normalizeScheduleTasksPayload({ tasks: [{ id: 'daily', llm_no: null }] }).tasks[0].llm_no, null)
  assert.equal(normalizeScheduleTasksPayload({ tasks: [{ id: 'daily', llm_no: -1 }] }).tasks[0].llm_no, null)
})

test('schedule UI exposes model selection, card editing, and grouped markdown reports', () => {
  const app = readFileSync(new URL('../App.jsx', import.meta.url), 'utf8')
  const component = readFileSync(new URL('../components/schedule.jsx', import.meta.url), 'utf8')
  const workbench = readFileSync(new URL('../components/ScheduledTaskWorkbench.jsx', import.meta.url), 'utf8')
  assert.match(app, /const known = \['enabled','max_delay_hours','repeat','schedule','prompt','llm_no'\]/)
  assert.match(app, /<ScheduleReportTree tasks=\{tasks\}/)
  assert.match(app, /schedulerModelNo/)
  assert.match(component, /followLabel/)
  assert.match(workbench, /aria-selected=\{selected\}/)
  assert.match(workbench, /onSelect=\{loadTask\}/)
  assert.match(component, /task-run-\$\{runState\}/)
  assert.match(component, /task\.latest_run\?\.reason \|\| task\.latest_run\?\.summary/)
  assert.match(component, /task-config-state/)
  assert.match(workbench, /task-run-\$\{runState\}/)
  assert.match(component, /Scheduler actual model/)
  assert.match(component, /Start with GA Admin/)
  assert.match(component, /is-selected/)
  assert.match(component, /aria-pressed=\{selected\}/)
  assert.ok(app.includes('onRun={runTask}'))
  assert.ok(app.includes('onReports={openTaskReports}'))
  assert.ok(app.includes('focusTaskId={scheduleReportTaskId}'))
  assert.match(component, /ScheduleArtifactPreview/)
  assert.match(component, /ProviderModelCascade/)
  assert.match(component, /buildModelProviderGroups\(llms/)
  assert.match(component, /disabledReason/)
  assert.ok(component.includes('className="task-run"'))
  assert.ok(component.includes('className="task-reports"'))
  assert.doesNotMatch(component, /mini-reports/)
})

test('latest run colors and mobile layout override legacy task status styles', () => {
  const style = readFileSync(new URL('../style.css', import.meta.url), 'utf8')
  assert.ok(style.lastIndexOf('.task-row.task-run-failed') > style.lastIndexOf('.task-row.status-overdue'))
  assert.match(style, /\.task-row\.task-run-failed \{[^}]*border-left-color:#dc2626 !important;[^}]*background:/)
  assert.match(style, /\.task-row\.task-run-blocked,\.task-row\.task-run-waiting,\.task-row\.task-run-partial/)
  assert.match(style, /@media \(max-width:640px\) \{[\s\S]*?\.task-card-head \{ display:grid; grid-template-columns:minmax\(0,1fr\); \}/)
  assert.match(style, /\.task-row \{ max-width:100%; overflow:hidden; \}/)
})

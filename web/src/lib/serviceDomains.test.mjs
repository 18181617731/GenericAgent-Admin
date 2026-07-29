import test from 'node:test'
import assert from 'node:assert/strict'
import { autonomousServices, goalWorkflowServices, guardianServices, scheduleServices } from './serviceDomains.js'

const services = [
  { name: 'reflect/scheduler.py', kind: 'reflect' },
  { name: 'reflect/autonomous.py', kind: 'reflect' },
  { name: 'reflect/custom_reflect.py', kind: 'reflect' },
  { name: 'reflect/agent_team_worker.py', kind: 'reflect' },
  { name: 'reflect/checklist_master.py', kind: 'reflect' },
  { name: 'reflect/watchdog.py', kind: 'guardian' },
  { name: 'reflect/goal_mode.py', kind: 'reflect' },
  { name: 'frontends/web.py', kind: 'frontend' },
]

test('scheduleServices keeps only scheduler controls on the scheduled-task page', () => {
  assert.deepEqual(scheduleServices(services).map(service => service.name), ['reflect/scheduler.py'])
})

test('autonomousServices excludes scheduler and Goal Mode services', () => {
  assert.deepEqual(autonomousServices(services).map(service => service.name), [
    'reflect/autonomous.py',
    'reflect/custom_reflect.py',
  ])
})

test('goalWorkflowServices keeps task-scoped Goal and checklist components together', () => {
  assert.deepEqual(goalWorkflowServices(services).map(service => service.name), [
    'reflect/agent_team_worker.py',
    'reflect/checklist_master.py',
  ])
})

test('guardianServices keeps watchdog out of model-backed reflect services', () => {
  assert.deepEqual(guardianServices(services).map(service => service.name), ['reflect/watchdog.py'])
})

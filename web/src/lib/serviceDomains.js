const serviceName = service => String(service?.name || '').toLowerCase()

const isReflectService = service => service?.kind === 'reflect' || serviceName(service).startsWith('reflect/')
const GOAL_WORKFLOW_NAMES = new Set(['reflect/agent_team_worker.py', 'reflect/checklist_master.py'])
const GUARDIAN_NAMES = new Set(['reflect/watchdog.py'])

export const scheduleServices = services => (Array.isArray(services) ? services : [])
  .filter(service => serviceName(service).includes('scheduler'))

export const autonomousServices = services => (Array.isArray(services) ? services : [])
  .filter(service => {
    const name = serviceName(service)
    return isReflectService(service) && !name.includes('scheduler') && !name.includes('goal_mode') && !GOAL_WORKFLOW_NAMES.has(name) && !GUARDIAN_NAMES.has(name)
  })

export const goalWorkflowServices = services => (Array.isArray(services) ? services : [])
  .filter(service => GOAL_WORKFLOW_NAMES.has(serviceName(service)))

export const guardianServices = services => (Array.isArray(services) ? services : [])
  .filter(service => service?.kind === 'guardian' || GUARDIAN_NAMES.has(serviceName(service)))

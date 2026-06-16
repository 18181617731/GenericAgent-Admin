export const NAV_ITEMS = ['overview','chat','control','files','tasks','pets','memory','channels','autonomous','goals','models','settings','logs']
export const ROUTE_TABS = NAV_ITEMS
export const TASK_SUB_TABS = ['services','scheduled','runs','reports']

const TAB_ALIASES = {
  '': 'overview',
  home: 'overview',
  index: 'overview',
  task: 'tasks',
  tasks: 'tasks',
  tmwebdriver: 'control',
  tmwd: 'control',
  config: 'settings',
  pet: 'pets',
  pets: 'pets',
  desktop_pet: 'pets',
  hatch_pet: 'pets',
}

const TASK_ROUTE_ALIASES = {
  '': 'services',
  service: 'services',
  services: 'services',
  schedule: 'scheduled',
  scheduled: 'scheduled',
  runs: 'runs',
  goals: 'runs',
  reports: 'reports',
}

const baseURL = () => (import.meta.env?.BASE_URL || '/').replace(/\/$/, '')

const routeParts = () => {
  const rawHash = (window.location.hash || '').replace(/^#\/?/, '').split('/').filter(Boolean)
  if (rawHash.length) return rawHash
  const base = baseURL()
  let path = window.location.pathname || '/'
  if (base && base !== '/' && path.startsWith(base)) path = path.slice(base.length) || '/'
  return path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean)
}

export const parseRoute = () => {
  const parts = routeParts()
  const rawFirst = parts[0] || ''
  const directTaskSubTab = TASK_ROUTE_ALIASES[rawFirst]
  const first = directTaskSubTab && rawFirst !== '' ? 'tasks' : (TAB_ALIASES[rawFirst] || rawFirst)
  const tab = ROUTE_TABS.includes(first) ? first : 'overview'
  const rawSub = tab === 'tasks' ? (parts[1] || (directTaskSubTab ? rawFirst : '')) : ''
  const sub = TASK_ROUTE_ALIASES[rawSub] || rawSub
  const taskSubTab = tab === 'tasks' && TASK_SUB_TABS.includes(sub) ? sub : 'services'
  return { tab, taskSubTab }
}

export const buildRoute = (tab, taskSubTab = 'services') => {
  const safeTab = ROUTE_TABS.includes(tab) ? tab : 'overview'
  const suffix = safeTab === 'tasks' ? `/${TASK_SUB_TABS.includes(taskSubTab) ? taskSubTab : 'services'}` : ''
  const base = baseURL()
  return `${base}/${safeTab}${suffix}`.replace(/\/+/g, '/')
}

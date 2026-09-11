export const NAV_ITEMS = ['overview','instances','chat','notifications','files','memory','local-cmd','channels','tasks','autonomous','usage','goals','models','settings','logs']
export const ROUTE_TABS = NAV_ITEMS
export const TASK_SUB_TABS = ['scheduled','reports']

const TAB_ALIASES = {
  '': 'overview',
  home: 'overview',
  index: 'overview',
  task: 'tasks',
  tasks: 'tasks',
  runs: 'goals',
  config: 'settings',
}

const TASK_ROUTE_ALIASES = {
  '': 'scheduled',
  service: 'scheduled',
  services: 'scheduled',
  schedule: 'scheduled',
  scheduled: 'scheduled',
  reports: 'reports',
}

const baseURL = () => (import.meta.env?.BASE_URL || '/').replace(/\/$/, '')

// The chat shell owns the root routes while the admin console is mounted below
// `/admin`. Keep the prefix in one place so direct links, refreshes, and
// client-side navigation all resolve to the same route family.
const routeBase = () => {
  const path = window.location.pathname || '/'
  if (path === '/admin' || path.startsWith('/admin/')) return '/admin'
  return baseURL()
}

const routeParts = () => {
  const rawHash = (window.location.hash || '').replace(/^#\/?/, '').split('/').filter(Boolean)
  if (rawHash.length) return rawHash
  const base = routeBase()
  let path = window.location.pathname || '/'
  if (base && base !== '/' && path.startsWith(base)) path = path.slice(base.length) || '/'
  return path.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean)
}

export const parseRoute = () => {
  const parts = routeParts()
  const rawFirst = parts[0] || ''
  const directTaskSubTab = ROUTE_TABS.includes(rawFirst) ? '' : TASK_ROUTE_ALIASES[rawFirst]
  const first = directTaskSubTab && rawFirst !== '' ? 'tasks' : (TAB_ALIASES[rawFirst] || rawFirst)
  let tab = ROUTE_TABS.includes(first) ? first : 'overview'
  if (tab === 'instances') return { tab, taskSubTab: 'services' }
  const rawSub = tab === 'tasks' ? (parts[1] || (directTaskSubTab ? rawFirst : '')) : ''
  if (tab === 'tasks' && (rawSub === 'runs' || rawSub === 'goals')) tab = 'goals'
  const sub = TASK_ROUTE_ALIASES[rawSub] || rawSub
  const taskSubTab = tab === 'tasks' && TASK_SUB_TABS.includes(sub) ? sub : 'scheduled'
  return { tab, taskSubTab }
}

export const buildRoute = (tab, taskSubTab = 'scheduled') => {
  const safeTab = ROUTE_TABS.includes(tab) ? tab : 'overview'
  const suffix = safeTab === 'tasks' ? `/${TASK_SUB_TABS.includes(taskSubTab) ? taskSubTab : 'scheduled'}` : ''
  // Chat is a separate root shell; navigating there from the admin console
  // must not create `/admin/chat`, which would be mounted by AdminRoot.
  const base = safeTab === 'chat' ? baseURL() : routeBase()
  return `${base}/${safeTab}${suffix}`.replace(/\/+/g, '/')
}

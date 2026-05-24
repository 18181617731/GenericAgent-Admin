export const NAV_ITEMS = ['overview','chat','control','files','tasks','bbs','memory','channels','autonomous','goals','models','settings','logs']
export const ROUTE_TABS = NAV_ITEMS.filter(n => n !== 'chat')
export const TASK_SUB_TABS = ['services','scheduled','runs','reports']

export const parseRoute = () => {
  const parts = (window.location.hash || '').replace(/^#\/?/, '').split('/').filter(Boolean)
  const tab = ROUTE_TABS.includes(parts[0]) ? parts[0] : 'overview'
  const taskSubTab = tab === 'tasks' && TASK_SUB_TABS.includes(parts[1]) ? parts[1] : 'services'
  return { tab, taskSubTab }
}

export const buildRoute = (tab, taskSubTab = 'services') => tab === 'tasks' ? `#/${tab}/${taskSubTab}` : `#/${tab}`

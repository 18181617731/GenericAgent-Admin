import { NAV_ITEMS } from './routing.js'

// The settings surface is grouped so the sidebar reads as a preferences list
// instead of a flat console menu. Every route in NAV_ITEMS belongs to exactly
// one group; settingsNavItems() is the single source of truth for nav order.
export const SETTINGS_GROUPS = [
  { id: 'general', items: ['overview', 'settings', 'chat'] },
  { id: 'agent', items: ['models', 'instances', 'channels'] },
  { id: 'automation', items: ['tasks', 'goals', 'autonomous'] },
  { id: 'system', items: ['files', 'usage', 'logs', 'notifications', 'memory'] },
]

export const settingsNavItems = () => SETTINGS_GROUPS.flatMap(group => group.items)

export const settingsNavGroupOf = (tab) => SETTINGS_GROUPS.find(group => group.items.includes(tab))?.id || ''

export const unassignedNavItems = () => {
  const assigned = new Set(settingsNavItems())
  return NAV_ITEMS.filter(item => !assigned.has(item))
}

import React, { useEffect, useMemo, useState } from 'react'
import { Bell, BellRing, Check, CheckCheck, Clock3, ExternalLink, Monitor, Save, Settings2, Trash2, Volume2 } from 'lucide-react'
import {
  NOTIFICATION_CATEGORIES,
  browserNotificationPermission,
  clearReadNotifications,
  formatNotificationTime,
  loadNotificationSettings,
  loadNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  notificationCategoryLabel,
  notificationLevelLabel,
  notificationPath,
  normalizeNotificationSettings,
  publishNotification,
  requestBrowserNotificationPermission,
  saveNotificationSettings,
  subscribeNotifications,
} from '../lib/notifications.js'

const COPY = {
  zh: {
    title: '消息通知', intro: '把对话、定时任务、自主进化和 Goal 模式的完成或异常集中提醒，避免错过后台执行结果。', inbox: '通知收件箱', settings: '通知设置', all: '全部', unread: '未读', empty: '暂无通知', markAll: '全部已读', clearRead: '清理已读', test: '发送测试通知', save: '保存设置', saved: '通知设置已保存', inApp: '站内通知', browser: '浏览器通知', browserHelp: '仅在浏览器允许后生效；可在后台标签页提醒。', request: '允许浏览器通知', permission: '当前权限', unsupported: '当前浏览器不支持', granted: '已允许', denied: '已拒绝', default: '未设置', sound: '完成时播放提示音', backgroundOnly: '仅在页面切到后台时显示浏览器通知', quiet: '静默时段', quietHelp: '静默时段内仍保留站内通知，但不播放声音、不弹浏览器通知。', start: '开始', end: '结束', categories: '通知分类', retention: '最多保留通知', items: '条', testTitle: '测试通知', testMessage: '消息通知功能工作正常。', open: '查看', detail: '点击通知可进入相关页面。', channelHelp: '站内通知始终作为主要渠道；其他渠道按需启用。', noPermission: '请先允许浏览器通知。', saveTip: '修改后点击保存，设置会保存在当前浏览器。', status: '状态', time: '时间', category: '分类', close: '关闭', notificationOn: '已开启', notificationOff: '已关闭', enable: '启用', disable: '停用', },
  en: {
    title: 'Notifications', intro: 'Keep chat, scheduled tasks, autonomous runs, and Goal Mode results visible without constantly checking each page.', inbox: 'Inbox', settings: 'Notification settings', all: 'All', unread: 'Unread', empty: 'No notifications', markAll: 'Mark all read', clearRead: 'Clear read', test: 'Send test notification', save: 'Save settings', saved: 'Notification settings saved', inApp: 'In-app notifications', browser: 'Browser notifications', browserHelp: 'Works after browser permission is granted; can notify while this tab is in the background.', request: 'Allow browser notifications', permission: 'Permission', unsupported: 'Not supported', granted: 'Granted', denied: 'Denied', default: 'Not set', sound: 'Play a completion sound', backgroundOnly: 'Only show browser notifications when the page is in background', quiet: 'Quiet hours', quietHelp: 'In-app notifications remain available during quiet hours, while sound and browser notifications are muted.', start: 'Start', end: 'End', categories: 'Notification categories', retention: 'Keep up to', items: 'items', testTitle: 'Test notification', testMessage: 'Notifications are working.', open: 'Open', detail: 'Select a notification to open its related page.', close: 'Close', notificationOn: 'Enabled', notificationOff: 'Disabled', enable: 'Enable', disable: 'Disable', saveTip: 'Save changes to keep them in this browser.', status: 'Status', time: 'Time', category: 'Category', channelHelp: 'In-app notifications are the primary channel; enable other channels when useful.', noPermission: 'Allow browser notifications first.', },
}

const levelClass = item => `notification-page-item is-${item.level}${item.read ? ' is-read' : ''}`

export function NotificationsPage({ lang = 'zh', onOpen }) {
  const c = COPY[lang] || COPY.zh
  const [settings, setSettings] = useState(() => loadNotificationSettings())
  const [items, setItems] = useState(() => loadNotifications())
  const [filter, setFilter] = useState('all')
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [saved, setSaved] = useState(true)
  const [message, setMessage] = useState('')
  const [permission, setPermission] = useState(browserNotificationPermission)

  useEffect(() => subscribeNotifications(() => { setItems(loadNotifications()); setSettings(loadNotificationSettings()) }), [])
  const visibleItems = useMemo(() => items.filter(item => (filter === 'all' || item.category === filter) && (!unreadOnly || !item.read)), [items, filter, unreadOnly])
  const patchSettings = patch => { setSettings(current => normalizeNotificationSettings({ ...current, ...patch })); setSaved(false); setMessage('') }
  const patchChannel = (key, value) => patchSettings({ channels: { ...settings.channels, [key]: value } })
  const patchQuiet = (key, value) => patchSettings({ quietHours: { ...settings.quietHours, [key]: value } })
  const patchCategory = (key, value) => patchSettings({ categories: { ...settings.categories, [key]: value } })
  const save = () => { const next = saveNotificationSettings(settings); setSettings(next); setSaved(true); setMessage(c.saved) }
  const readAll = () => { markAllNotificationsRead(); setItems(loadNotifications()) }
  const clearRead = () => { clearReadNotifications(); setItems(loadNotifications()) }
  const openItem = item => {
    markNotificationRead(item.id)
    setItems(loadNotifications())
    if (onOpen) onOpen(item)
    else window.location.href = notificationPath(item)
  }
  const requestPermission = async () => {
    const next = await requestBrowserNotificationPermission()
    setPermission(next)
    if (next === 'granted') patchChannel('browser', true)
  }
  const sendTest = () => { saveNotificationSettings(settings); publishNotification({ category: 'system', level: 'info', title: c.testTitle, message: c.testMessage, route: 'notifications', dedupeKey: `notification-test:${Date.now()}` }); setItems(loadNotifications()) }
  const permissionLabel = { granted: c.granted, denied: c.denied, default: c.default, unsupported: c.unsupported }[permission] || permission
  return <section className="notifications-page">
    <div className="notifications-intro"><div><span className="eyebrow"><Bell size={15}/>{c.title}</span><p>{c.intro}</p></div><div className="notifications-intro-actions"><button type="button" onClick={sendTest}><BellRing size={15}/>{c.test}</button><button type="button" className="primary" onClick={save} disabled={saved}><Save size={15}/>{c.save}</button></div></div>
    <div className="notifications-layout">
      <section className="notification-inbox panel"><div className="notification-section-head"><div><h2>{c.inbox}</h2><p>{c.detail}</p></div><div className="notification-inbox-actions"><button type="button" onClick={readAll} disabled={!items.some(item => !item.read)} title={c.markAll}><CheckCheck size={15}/>{c.markAll}</button><button type="button" onClick={clearRead} disabled={!items.some(item => item.read)} title={c.clearRead}><Trash2 size={15}/>{c.clearRead}</button></div></div>
        <div className="notification-filters"><div className="notification-filter-scroll"><button type="button" className={filter === 'all' ? 'active' : ''} onClick={() => setFilter('all')}>{c.all}<span>{items.length}</span></button>{NOTIFICATION_CATEGORIES.map(category => <button type="button" key={category.id} className={filter === category.id ? 'active' : ''} onClick={() => setFilter(category.id)}>{notificationCategoryLabel(category.id, lang)}<span>{items.filter(item => item.category === category.id).length}</span></button>)}</div><label className="notification-unread-toggle"><input type="checkbox" checked={unreadOnly} onChange={event => setUnreadOnly(event.target.checked)}/>{c.unread}</label></div>
        <div className="notification-page-list">{visibleItems.length ? visibleItems.map(item => <button type="button" key={item.id} className={levelClass(item)} onClick={() => openItem(item)}><span className="notification-level-dot"/><span className="notification-page-copy"><strong>{item.title}</strong><span>{item.message}</span><small>{notificationCategoryLabel(item.category, lang)} · {formatNotificationTime(item.createdAt, lang)}</small></span><span className="notification-page-status">{notificationLevelLabel(item.level, lang)}<ExternalLink size={14}/></span></button>) : <div className="notification-page-empty"><Bell size={26}/><b>{c.empty}</b><span>{c.detail}</span></div>}</div>
      </section>
      <aside className="notification-settings panel"><div className="notification-section-head"><div><h2><Settings2 size={17}/>{c.settings}</h2><p>{c.saveTip}</p></div><span className={`notification-settings-state ${settings.enabled ? 'is-on' : 'is-off'}`}>{settings.enabled ? c.notificationOn : c.notificationOff}</span></div>
        <label className="notification-switch-row"><span><b>{c.inApp}</b><small>{c.channelHelp}</small></span><input type="checkbox" checked={settings.enabled && settings.channels.inApp} onChange={event => patchSettings({ enabled: event.target.checked, channels: { ...settings.channels, inApp: event.target.checked } })}/></label>
        <div className="notification-setting-group"><h3><Monitor size={15}/>{c.browser}</h3><p>{c.browserHelp}</p><div className="notification-permission"><span>{c.permission}：{permissionLabel}</span><button type="button" onClick={requestPermission} disabled={permission === 'unsupported' || permission === 'granted'}>{permission === 'granted' ? c.granted : c.request}</button></div><label className="notification-check-row"><input type="checkbox" checked={settings.channels.browser} onChange={event => patchChannel('browser', event.target.checked)} disabled={permission !== 'granted'}/><span>{c.browser}</span></label><label className="notification-check-row"><input type="checkbox" checked={settings.channels.backgroundOnly} onChange={event => patchChannel('backgroundOnly', event.target.checked)}/><span>{c.backgroundOnly}</span></label>{permission === 'denied' && <small className="notification-warning">{c.noPermission}</small>}</div>
        <div className="notification-setting-group"><h3><Volume2 size={15}/>{c.sound}</h3><label className="notification-check-row"><input type="checkbox" checked={settings.channels.sound} onChange={event => patchChannel('sound', event.target.checked)}/><span>{c.sound}</span></label></div>
        <div className="notification-setting-group"><h3><Clock3 size={15}/>{c.quiet}</h3><p>{c.quietHelp}</p><label className="notification-check-row"><input type="checkbox" checked={settings.quietHours.enabled} onChange={event => patchQuiet('enabled', event.target.checked)}/><span>{c.enable}</span></label><div className="notification-time-row"><label>{c.start}<input type="time" value={settings.quietHours.start} onChange={event => patchQuiet('start', event.target.value)}/></label><label>{c.end}<input type="time" value={settings.quietHours.end} onChange={event => patchQuiet('end', event.target.value)}/></label></div></div>
        <div className="notification-setting-group"><h3><Bell size={15}/>{c.categories}</h3><div className="notification-category-settings">{NOTIFICATION_CATEGORIES.map(category => <label className="notification-check-row" key={category.id}><input type="checkbox" checked={settings.categories[category.id] !== false} onChange={event => patchCategory(category.id, event.target.checked)}/><span><b>{notificationCategoryLabel(category.id, lang)}</b><small>{lang === 'en' ? category.description.replace('对话完成、失败或被中止', 'Completion, failure, or cancellation') : category.description}</small></span></label>)}</div></div>
        <label className="notification-retention"><span>{c.retention}</span><select value={settings.maxItems} onChange={event => patchSettings({ maxItems: Number(event.target.value) })}><option value="50">50 {c.items}</option><option value="100">100 {c.items}</option><option value="200">200 {c.items}</option></select></label>
        {message && <p className="notification-settings-message" role="status"><Check size={14}/>{message}</p>}
      </aside>
    </div>
  </section>
}

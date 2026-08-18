import React, { useEffect, useMemo, useState } from 'react'
import { AlertTriangle, Bell, BellRing, Check, CheckCheck, Clock3, ExternalLink, Monitor, Save, Settings2, Trash2, Volume2 } from 'lucide-react'
import { playChatCompletionTone } from '../lib/chatCompletionTone.js'
import { chatNotificationForDisplay } from '../lib/chatPrivacy.js'
import { useChatPrivacyMode } from '../hooks/useChatPrivacyMode.js'
import {
  NOTIFICATION_CATEGORIES,
  browserNotificationCapability,
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
    title: '消息通知', intro: '把对话、定时任务、自主进化和 Goal 模式的完成或异常集中提醒，避免错过后台执行结果。', inbox: '通知收件箱', settings: '通知设置', all: '全部', unread: '未读', empty: '暂无通知', markAll: '全部已读', clearRead: '清理已读', test: '发送测试通知', save: '保存设置', saved: '通知设置已保存', inApp: '站内通知', browser: '浏览器通知', browserHelp: '浏览器通知需要 HTTPS（localhost/127.0.0.1 可例外）；iPhone Chrome 普通标签页通常需要先添加到主屏幕。', request: '允许浏览器通知', permission: '当前状态', unsupported: '当前浏览器不支持', granted: '已允许', denied: '已拒绝', default: '未设置', insecure: '当前地址不是安全连接，请改用 HTTPS。', iosBrowser: 'iPhone Chrome 普通标签页不能可靠接收系统通知，请使用 HTTPS 并添加到主屏幕后重试。', sound: '完成时播放提示音', backgroundOnly: '仅在页面切到后台时显示浏览器通知', quiet: '静默时段', quietHelp: '静默时段内仍保留站内通知，但不播放声音、不弹浏览器通知。', start: '开始', end: '结束', categories: '通知分类', retention: '最多保留通知', items: '条', testTitle: '测试通知', testMessage: '消息通知功能工作正常。', testSent: '测试通知已发送，正在请求播放提示音；若仍无声，请检查 iPhone 静音开关和媒体音量。', testSoundOff: '测试通知已发送，但“完成时播放提示音”已关闭。', testSoundFailed: '测试通知已发送，但浏览器未能播放提示音，请检查 iPhone 静音开关和媒体音量后重试。', open: '查看', detail: '点击通知可进入相关页面。', channelHelp: '站内通知始终作为主要渠道；其他渠道按需启用。', noPermission: '浏览器已拒绝通知，请到当前网站的浏览器权限中改为允许，再回到此页面刷新。', permissionHelp: '点击按钮后，浏览器会弹出权限请求；如果没有弹窗，请检查地址是否为 HTTPS。', grantedHelp: '浏览器权限已允许，请勾选下方“浏览器通知”并保存设置。', close: '关闭', notificationOn: '已开启', notificationOff: '已关闭', enable: '启用', disable: '停用', },
  en: {
    title: 'Notifications', intro: 'Keep chat, scheduled tasks, autonomous runs, and Goal Mode results visible without constantly checking each page.', inbox: 'Inbox', settings: 'Notification settings', all: 'All', unread: 'Unread', empty: 'No notifications', markAll: 'Mark all read', clearRead: 'Clear read', test: 'Send test notification', save: 'Save settings', saved: 'Notification settings saved', inApp: 'In-app notifications', browser: 'Browser notifications', browserHelp: 'Browser notifications require HTTPS (localhost/127.0.0.1 is exempt); iPhone Chrome usually requires adding this page to the Home Screen first.', request: 'Allow browser notifications', permission: 'Status', unsupported: 'Not supported', granted: 'Granted', denied: 'Denied', default: 'Not set', insecure: 'This address is not secure. Use HTTPS to enable browser notifications.', iosBrowser: 'Regular iPhone Chrome tabs cannot reliably receive system notifications. Use HTTPS and add this page to the Home Screen, then try again.', sound: 'Play a completion sound', backgroundOnly: 'Only show browser notifications when the page is in background', quiet: 'Quiet hours', quietHelp: 'In-app notifications remain available during quiet hours, while sound and browser notifications are muted.', start: 'Start', end: 'End', categories: 'Notification categories', retention: 'Keep up to', items: 'items', testTitle: 'Test notification', testMessage: 'Notifications are working.', testSent: 'Test notification sent. Playback was requested; if silent, check the iPhone mute switch and media volume.', testSoundOff: 'Test notification sent, but the completion sound is disabled.', testSoundFailed: 'Test notification sent, but the browser could not play the sound. Check the iPhone mute switch and media volume, then retry.', open: 'Open', detail: 'Select a notification to open its related page.', close: 'Close', notificationOn: 'Enabled', notificationOff: 'Disabled', enable: 'Enable', disable: 'Disable', saveTip: 'Save changes to keep them in this browser.', status: 'Status', time: 'Time', category: 'Category', channelHelp: 'In-app notifications are the primary channel; enable other channels when useful.', noPermission: 'The browser denied notifications. Change this site permission to Allow, then return and refresh.', permissionHelp: 'Click the button to request permission. If no prompt appears, check that the address uses HTTPS.', grantedHelp: 'Permission is granted. Enable Browser notifications below and save settings.', },
}

const levelClass = item => `notification-page-item is-${item.level}${item.read ? ' is-read' : ''}`

const browserPermissionLabel = (capability, copy) => {
  if (capability.status === 'insecure') return copy.insecure
  if (capability.status === 'ios-browser') return copy.iosBrowser
  return { granted: copy.granted, denied: copy.denied, default: copy.default, unsupported: copy.unsupported }[capability.permission] || capability.permission
}

const browserFeedback = (result, capability, copy) => {
  if (result === 'insecure' || capability.status === 'insecure') return copy.insecure
  if (result === 'ios-browser' || capability.status === 'ios-browser') return copy.iosBrowser
  if (result === 'unsupported' || capability.status === 'unsupported') return copy.unsupported
  if (result === 'denied' || capability.permission === 'denied') return copy.noPermission
  return copy.permissionHelp
}

export function NotificationsPage({ lang = 'zh', onOpen }) {
  const c = COPY[lang] || COPY.zh
  const [privacyMode] = useChatPrivacyMode()
  const [settings, setSettings] = useState(() => loadNotificationSettings())
  const [items, setItems] = useState(() => loadNotifications())
  const [filter, setFilter] = useState('all')
  const [unreadOnly, setUnreadOnly] = useState(false)
  const [saved, setSaved] = useState(true)
  const [message, setMessage] = useState('')
  const [messageTone, setMessageTone] = useState('success')
  const [browserCapability, setBrowserCapability] = useState(() => browserNotificationCapability())

  useEffect(() => subscribeNotifications(() => { setItems(loadNotifications()); setSettings(loadNotificationSettings()) }), [])
  useEffect(() => {
    const refresh = () => setBrowserCapability(browserNotificationCapability())
    window.addEventListener('focus', refresh)
    document.addEventListener('visibilitychange', refresh)
    return () => { window.removeEventListener('focus', refresh); document.removeEventListener('visibilitychange', refresh) }
  }, [])
  const visibleItems = useMemo(() => items.filter(item => (filter === 'all' || item.category === filter) && (!unreadOnly || !item.read)), [items, filter, unreadOnly])
  const patchSettings = patch => { setSettings(current => normalizeNotificationSettings({ ...current, ...patch })); setSaved(false); setMessage(''); setMessageTone('success') }
  const patchChannel = (key, value) => patchSettings({ channels: { ...settings.channels, [key]: value } })
  const patchQuiet = (key, value) => patchSettings({ quietHours: { ...settings.quietHours, [key]: value } })
  const patchCategory = (key, value) => patchSettings({ categories: { ...settings.categories, [key]: value } })
  const save = () => { const next = saveNotificationSettings(settings); setSettings(next); setSaved(true); setMessage(c.saved); setMessageTone('success') }
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
    const capability = browserNotificationCapability()
    const resolved = ['unsupported', 'insecure', 'ios-browser'].includes(next) ? { ...capability, status: next } : capability
    setBrowserCapability(resolved)
    if (next === 'granted') {
      patchChannel('browser', true)
      setMessage(c.grantedHelp)
      setMessageTone('success')
    } else {
      setMessage(browserFeedback(next, resolved, c))
      setMessageTone('warning')
    }
  }
  const sendTest = () => {
    const persisted = saveNotificationSettings(settings)
    const soundEnabled = persisted.channels.sound && persisted.categories.system !== false
    if (soundEnabled) {
      const started = playChatCompletionTone({ onError: () => { setMessage(c.testSoundFailed); setMessageTone('warning') } })
      setMessage(started ? c.testSent : c.testSoundFailed)
      setMessageTone(started ? 'success' : 'warning')
    } else {
      setMessage(c.testSoundOff)
      setMessageTone('warning')
    }
    publishNotification({ category: 'system', level: 'info', title: c.testTitle, message: c.testMessage, route: 'notifications', dedupeKey: `notification-test:${Date.now()}` }, { playSound:false })
    setItems(loadNotifications())
  }
  const permission = browserCapability.permission
  const permissionLabel = browserPermissionLabel(browserCapability, c)
  return <section className="notifications-page">
    <div className="notifications-intro"><div><span className="eyebrow"><Bell size={15}/>{c.title}</span><p>{c.intro}</p></div><div className="notifications-intro-actions"><button type="button" onClick={sendTest}><BellRing size={15}/>{c.test}</button><button type="button" className="primary" onClick={save} disabled={saved}><Save size={15}/>{c.save}</button></div></div>
    <div className="notifications-layout">
      <section className="notification-inbox panel"><div className="notification-section-head"><div><h2>{c.inbox}</h2><p>{c.detail}</p></div><div className="notification-inbox-actions"><button type="button" onClick={readAll} disabled={!items.some(item => !item.read)} title={c.markAll}><CheckCheck size={15}/>{c.markAll}</button><button type="button" onClick={clearRead} disabled={!items.some(item => item.read)} title={c.clearRead}><Trash2 size={15}/>{c.clearRead}</button></div></div>
        <div className="notification-filters"><div className="notification-filter-scroll"><button type="button" className={filter === 'all' ? 'active' : ''} onClick={() => setFilter('all')}>{c.all}<span>{items.length}</span></button>{NOTIFICATION_CATEGORIES.map(category => <button type="button" key={category.id} className={filter === category.id ? 'active' : ''} onClick={() => setFilter(category.id)}>{notificationCategoryLabel(category.id, lang)}<span>{items.filter(item => item.category === category.id).length}</span></button>)}</div><label className="notification-unread-toggle"><input type="checkbox" checked={unreadOnly} onChange={event => setUnreadOnly(event.target.checked)}/>{c.unread}</label></div>
        <div className="notification-page-list">{visibleItems.length ? visibleItems.map(item => {
          const displayItem = chatNotificationForDisplay(item, privacyMode, lang)
          return <button type="button" key={item.id} className={levelClass(item)} onClick={() => openItem(item)}><span className="notification-level-dot"/><span className="notification-page-copy"><strong>{displayItem.title}</strong><span>{displayItem.message}</span><small>{notificationCategoryLabel(item.category, lang)} · {formatNotificationTime(item.createdAt, lang)}</small></span><span className="notification-page-status">{notificationLevelLabel(item.level, lang)}<ExternalLink size={14}/></span></button>
        }) : <div className="notification-page-empty"><Bell size={26}/><b>{c.empty}</b><span>{c.detail}</span></div>}</div>
      </section>
      <aside className="notification-settings panel"><div className="notification-section-head"><div><h2><Settings2 size={17}/>{c.settings}</h2><p>{c.saveTip}</p></div><span className={`notification-settings-state ${settings.enabled ? 'is-on' : 'is-off'}`}>{settings.enabled ? c.notificationOn : c.notificationOff}</span></div>
        <label className="notification-switch-row"><span><b>{c.inApp}</b><small>{c.channelHelp}</small></span><input type="checkbox" checked={settings.enabled && settings.channels.inApp} onChange={event => patchSettings({ enabled: event.target.checked, channels: { ...settings.channels, inApp: event.target.checked } })}/></label>
        <div className="notification-setting-group"><h3><Monitor size={15}/>{c.browser}</h3><p>{c.browserHelp}</p><p className={`notification-browser-compatibility is-${browserCapability.status}`} role="status">{browserPermissionLabel(browserCapability, c)}</p><div className="notification-permission"><span>{c.permission}：{permissionLabel}</span><button type="button" onClick={requestPermission} disabled={permission === 'granted'}>{permission === 'granted' ? c.granted : c.request}</button></div><label className="notification-check-row"><input type="checkbox" checked={settings.channels.browser} onChange={event => patchChannel('browser', event.target.checked)} disabled={permission !== 'granted'}/><span>{c.browser}</span></label><label className="notification-check-row"><input type="checkbox" checked={settings.channels.backgroundOnly} onChange={event => patchChannel('backgroundOnly', event.target.checked)}/><span>{c.backgroundOnly}</span></label>{permission === 'denied' && browserCapability.status === 'available' && <small className="notification-warning">{c.noPermission}</small>}</div>
        <div className="notification-setting-group"><h3><Volume2 size={15}/>{c.sound}</h3><label className="notification-check-row"><input type="checkbox" checked={settings.channels.sound} onChange={event => patchChannel('sound', event.target.checked)}/><span>{c.sound}</span></label></div>
        <div className="notification-setting-group"><h3><Clock3 size={15}/>{c.quiet}</h3><p>{c.quietHelp}</p><label className="notification-check-row"><input type="checkbox" checked={settings.quietHours.enabled} onChange={event => patchQuiet('enabled', event.target.checked)}/><span>{c.enable}</span></label><div className="notification-time-row"><label>{c.start}<input type="time" value={settings.quietHours.start} onChange={event => patchQuiet('start', event.target.value)}/></label><label>{c.end}<input type="time" value={settings.quietHours.end} onChange={event => patchQuiet('end', event.target.value)}/></label></div></div>
        <div className="notification-setting-group"><h3><Bell size={15}/>{c.categories}</h3><div className="notification-category-settings">{NOTIFICATION_CATEGORIES.map(category => <label className="notification-check-row" key={category.id}><input type="checkbox" checked={settings.categories[category.id] !== false} onChange={event => patchCategory(category.id, event.target.checked)}/><span><b>{notificationCategoryLabel(category.id, lang)}</b><small>{lang === 'en' ? category.description.replace('对话完成、失败或被中止', 'Completion, failure, or cancellation') : category.description}</small></span></label>)}</div></div>
        <label className="notification-retention"><span>{c.retention}</span><select value={settings.maxItems} onChange={event => patchSettings({ maxItems: Number(event.target.value) })}><option value="50">50 {c.items}</option><option value="100">100 {c.items}</option><option value="200">200 {c.items}</option></select></label>
        {message && <p className={`notification-settings-message is-${messageTone}`} role="status">{messageTone === 'warning' ? <AlertTriangle size={14}/> : <Check size={14}/>} {message}</p>}
      </aside>
    </div>
  </section>
}

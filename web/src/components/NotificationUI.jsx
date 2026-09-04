import { useEffect, useRef, useState } from 'react'
import { Bell, BellRing, CheckCheck, ExternalLink, X } from 'lucide-react'
import {
  formatNotificationTime,
  loadNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  notificationCategoryLabel,
  notificationLevelLabel,
  notificationPath,
  subscribeNotifications,
  unreadNotificationCount,
} from '../lib/notifications.js'
import { chatNotificationForDisplay } from '../lib/chatPrivacy.js'
import { useChatPrivacyMode } from '../hooks/useChatPrivacyMode.js'

const COPY = {
  zh: { title: '消息通知', empty: '暂无通知', unread: '未读', allRead: '全部已读', viewAll: '查看全部通知', close: '关闭', open: '打开相关页面' },
  en: { title: 'Notifications', empty: 'No notifications', unread: 'unread', allRead: 'Mark all read', viewAll: 'View all notifications', close: 'Close', open: 'Open related page' },
}
const itemClass = item => `notification-item is-${item.level}${item.read ? ' is-read' : ''}`

export function NotificationCenter({ lang = 'zh', onOpen }) {
  const copy = COPY[lang] || COPY.zh
  const [items, setItems] = useState(() => loadNotifications())
  const [open, setOpen] = useState(false)
  const [toast, setToast] = useState(null)
  const [privacyMode] = useChatPrivacyMode()
  const toastTimer = useRef(null)
  const unread = unreadNotificationCount(items)

  useEffect(() => {
    const receive = event => {
      setItems(loadNotifications())
      if (event?.kind !== 'item' || !event.item) return
      setToast(event.item)
      window.clearTimeout(toastTimer.current)
      toastTimer.current = window.setTimeout(() => setToast(null), 6500)
    }
    const unsubscribe = subscribeNotifications(receive)
    return () => { unsubscribe(); window.clearTimeout(toastTimer.current) }
  }, [])

  useEffect(() => {
    if (!open) return undefined
    const closeOnEscape = event => {
      if (event.key === 'Escape') setOpen(false)
    }
    const closeOnOutsidePointer = event => {
      if (!event.target.closest?.('.notification-center')) setOpen(false)
    }
    document.addEventListener('keydown', closeOnEscape)
    document.addEventListener('pointerdown', closeOnOutsidePointer)
    return () => {
      document.removeEventListener('keydown', closeOnEscape)
      document.removeEventListener('pointerdown', closeOnOutsidePointer)
    }
  }, [open])

  const openItem = item => {
    markNotificationRead(item.id)
    setItems(loadNotifications())
    setOpen(false)
    setToast(null)
    if (onOpen) onOpen(item)
    else window.location.href = notificationPath(item)
  }

  const readAll = () => { markAllNotificationsRead(); setItems(loadNotifications()) }
  const recent = items.slice(0, 6)
  return <>
    <div className="notification-center">
      <button type="button" className={`notification-bell${open ? ' is-open' : ''}`} aria-label={`${copy.title}${unread ? `，${unread} ${copy.unread}` : ''}`} aria-expanded={open} onClick={() => setOpen(value => !value)} title={copy.title}>
        {unread ? <BellRing size={17}/> : <Bell size={17}/>} {unread > 0 && <span className="notification-badge">{unread > 99 ? '99+' : unread}</span>}
      </button>
      {open && <div className="notification-popover" role="dialog" aria-label={copy.title}>
        <header><div><b>{copy.title}</b><span>{unread ? `${unread} ${copy.unread}` : copy.empty}</span></div><button type="button" onClick={readAll} disabled={!unread} title={copy.allRead}><CheckCheck size={16}/></button><button type="button" className="notification-popover-close" onClick={() => setOpen(false)} aria-label={copy.close} title={copy.close}><X size={16}/></button></header>
        <div className="notification-popover-list">{recent.length ? recent.map(item => {
          const displayItem = chatNotificationForDisplay(item, privacyMode, lang)
          return <button type="button" key={item.id} className={itemClass(item)} onClick={() => openItem(item)}>
            <span className="notification-level-dot"/><span className="notification-item-copy"><strong>{displayItem.title}</strong><small>{displayItem.message}</small><em>{notificationCategoryLabel(item.category, lang)} · {formatNotificationTime(item.createdAt, lang)}</em></span><ExternalLink size={14}/>
          </button>
        }) : <p className="notification-empty">{copy.empty}</p>}</div>
        <button type="button" className="notification-view-all" onClick={() => { setOpen(false); onOpen ? onOpen({ route: 'notifications' }) : (window.location.href = '/notifications') }}>{copy.viewAll}</button>
      </div>}
    </div>
    {toast && <div className={`notification-toast is-${toast.level}`} role="status" aria-live="polite">
      <button type="button" className="notification-toast-main" onClick={() => openItem(toast)}><span className="notification-level-dot"/><span><strong>{chatNotificationForDisplay(toast, privacyMode, lang).title}</strong><small>{chatNotificationForDisplay(toast, privacyMode, lang).message}</small></span></button>
      <span className="notification-toast-meta">{notificationLevelLabel(toast.level, lang)}</span><button type="button" className="notification-toast-close" onClick={() => setToast(null)} aria-label={copy.close}><X size={15}/></button>
    </div>}
  </>
}

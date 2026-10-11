import React, { lazy, Suspense, useState } from 'react'
import { createPortal } from 'react-dom'
import { ArrowUpRight, Download, X } from 'lucide-react'
import { I18N } from '../lib/i18n'
import { useVersionNotification } from '../hooks/useVersionNotification'
import { useVersionUpdates } from '../hooks/useVersionUpdates'
import './VersionUpdateNotification.css'

const SystemUpdateModal = lazy(() => import('./SystemUpdateModal'))

const noop = () => {}
export default function VersionUpdateNotification({ lang = 'zh' }) {
  const notice = useVersionNotification()
  const [open, setOpen] = useState(false)
  const version = useVersionUpdates({ t: I18N[lang], lang, setMsg: noop, setBusy: noop, active: open })
  const zh = lang !== 'en'
  const review = () => {
    version.loadUpdateSnapshot(notice.check)
    notice.dismiss()
    setOpen(true)
  }
  return <>
    {notice.release && !open && createPortal(<aside className="version-update-notice" aria-label={zh ? '新版本提醒' : 'New version available'}>
      <Download size={19} className="version-update-notice-icon" aria-hidden="true"/>
      <div className="version-update-notice-content">
        <div role="status"><strong>{zh ? '发现新版本' : 'New version available'}</strong><span>{notice.release}</span></div>
        <p>{zh ? '可查看更新内容，不会自动安装。' : 'Review what changed. Nothing is installed automatically.'}</p>
        <div className="version-update-notice-actions">
          <button type="button" className="version-update-notice-review" onClick={review}>{zh ? '查看更新' : 'View update'}<ArrowUpRight size={14}/></button>
          <button type="button" onClick={() => notice.setEnabled(false)}>{zh ? '关闭自动提醒' : 'Turn off reminders'}</button>
        </div>
      </div>
      <button type="button" className="version-update-notice-close" aria-label={zh ? '关闭此版本提醒' : 'Dismiss this version'} title={zh ? '此版本不再提醒，下个新版本仍会提醒' : 'Dismiss this version; future releases will still be announced'} onClick={notice.dismiss}><X size={16}/></button>
    </aside>, document.body)}
    {open && <Suspense fallback={null}><SystemUpdateModal open={open} onClose={() => setOpen(false)} version={version} lang={lang}/></Suspense>}
  </>
}

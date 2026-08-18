import { useCallback, useEffect, useState } from 'react'
import { loadChatPrivacyMode, saveChatPrivacyMode, subscribeChatPrivacyMode } from '../lib/chatPrivacy.js'

export function useChatPrivacyMode() {
  const [enabled, setEnabledState] = useState(loadChatPrivacyMode)

  useEffect(() => subscribeChatPrivacyMode(setEnabledState), [])

  const setEnabled = useCallback(value => {
    const next = typeof value === 'function' ? value(loadChatPrivacyMode()) : value
    return saveChatPrivacyMode(next)
  }, [])

  return [enabled, setEnabled]
}

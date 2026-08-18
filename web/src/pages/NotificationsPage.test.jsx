import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { NotificationsPage } from './NotificationsPage.jsx'
import { loadNotifications, publishNotification, saveNotificationSettings } from '../lib/notifications.js'
import { saveChatPrivacyMode } from '../lib/chatPrivacy.js'

afterEach(() => {
  cleanup()
  window.localStorage.clear()
})

describe('notifications page', () => {
  test('redacts existing chat notifications while privacy mode is enabled', () => {
    saveNotificationSettings({ channels:{ sound:false } })
    publishNotification({ category:'chat', level:'error', title:'SECRET_CHAT_TITLE', message:'SECRET_PROMPT D:/secret.txt', dedupeKey:'private-chat-test' })
    saveChatPrivacyMode(true)
    render(<NotificationsPage lang="zh" onOpen={vi.fn()}/>)

    expect(screen.getByText('对话任务失败')).toBeTruthy()
    expect(screen.getByText('隐私模式已隐藏会话标题和内容。')).toBeTruthy()
    expect(document.body.innerHTML).not.toContain('SECRET_CHAT_TITLE')
    expect(document.body.innerHTML).not.toContain('SECRET_PROMPT')
    expect(document.body.innerHTML).not.toContain('D:/secret.txt')
  })

  test('sends a test notification and renders it in the inbox', async () => {
    const onOpen = vi.fn()
    saveNotificationSettings({ channels: { sound: false } })
    render(<NotificationsPage lang="zh" onOpen={onOpen}/>)

    fireEvent.click(screen.getByRole('button', { name: '发送测试通知' }))
    expect(await screen.findByText('测试通知')).toBeTruthy()
    expect(screen.getByText('消息通知功能工作正常。')).toBeTruthy()
    const testItems = screen.getAllByRole('button', { name: /测试通知/ })
    fireEvent.click(testItems[testItems.length - 1])
    await waitFor(() => expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ route: 'notifications' })))
  })

  test('filters unread items and saves channel settings', async () => {
    saveNotificationSettings({ channels: { sound: false } })
    publishNotification({ category: 'goal', title: 'Goal 完成', message: '已完成', dedupeKey: 'goal-test' })
    render(<NotificationsPage lang="zh" onOpen={vi.fn()}/>)
    expect(screen.getByText('Goal 完成')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /全部已读/ }))
    fireEvent.click(screen.getByRole('checkbox', { name: '未读' }))
    expect(screen.getByText('暂无通知')).toBeTruthy()
    const sound = screen.getAllByRole('checkbox', { name: '完成时播放提示音' })[0]
    fireEvent.click(sound)
    fireEvent.click(screen.getByRole('button', { name: '保存设置' }))
    await waitFor(() => expect(screen.getByText('通知设置已保存')).toBeTruthy())
    expect(loadNotifications()).toHaveLength(1)
  })

  test('shows an immediate explanation when browser notification permission is unavailable', async () => {
    render(<NotificationsPage lang="zh" onOpen={vi.fn()}/>)
    fireEvent.click(screen.getByRole('button', { name: '允许浏览器通知' }))
    expect(await screen.findByText('当前浏览器不支持')).toBeTruthy()
  })
})

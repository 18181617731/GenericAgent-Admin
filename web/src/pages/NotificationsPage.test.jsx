import React from 'react'
import { afterEach, describe, expect, test, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { NotificationsPage } from './NotificationsPage.jsx'
import { loadNotifications, publishNotification, saveNotificationSettings } from '../lib/notifications.js'

afterEach(() => {
  cleanup()
  window.localStorage.clear()
})

describe('notifications page', () => {
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
})

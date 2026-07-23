import { apiRequest } from './api'

export async function listNotifications({ signal } = {}) {
  const payload = await apiRequest('/api/notifications', { fallbackMessage: 'Unable to load notifications.', signal })
  const notifications = payload?.data?.notifications
  const unreadCount = payload?.data?.unreadCount
  const limit = payload?.data?.window?.limit
  if (!Array.isArray(notifications)) {
    throw new Error('The notification response was incomplete. Reload to retry.')
  }
  if (unreadCount === undefined && limit === undefined) {
    return { notifications, unreadCount: await getNotificationUnreadCount({ signal }), limit: 50 }
  }
  if (!Number.isInteger(unreadCount) || unreadCount < 0 || !Number.isInteger(limit) || limit < 1) {
    throw new Error('The notification response was incomplete. Reload to retry.')
  }
  return { notifications, unreadCount, limit }
}

export async function getNotificationUnreadCount({ signal } = {}) {
  const payload = await apiRequest('/api/notifications/unread-count', { fallbackMessage: 'Unable to count notifications.', signal })
  return payload?.data?.unreadCount ?? 0
}

export async function markNotificationRead(notificationID, { signal } = {}) {
  await apiRequest(`/api/notifications/${notificationID}/read`, { method: 'PATCH', fallbackMessage: 'Unable to mark notification read.', signal })
}

export async function markAllNotificationsRead({ signal } = {}) {
  await apiRequest('/api/notifications/read-all', { method: 'POST', fallbackMessage: 'Unable to mark notifications read.', signal })
}

import { apiRequest } from './api'

export async function listNotifications({ signal } = {}) {
  const payload = await apiRequest('/api/notifications', { fallbackMessage: 'Unable to load notifications.', signal })
  return payload?.data?.notifications || []
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

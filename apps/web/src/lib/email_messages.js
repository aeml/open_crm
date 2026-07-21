import { apiRequest } from './api'

export function formatEmailTimestamp(value) {
  if (!value) return ''
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
}

export function emailMessageTimestamp(message) {
  return message?.receivedAt || message?.createdAt
}

export function emailRecordPath(message) {
  if (!message?.entityType || !message?.entityId) return ''
  const route = { contact: 'contacts', company: 'companies', deal: 'deals' }[message.entityType]
  return route ? `/${route}/${message.entityId}` : ''
}

export function emailRecordLabel(message) {
  return message?.entityType && message?.entityId ? `${message.entityType} #${message.entityId}` : ''
}

export function emailEngagementSummary(message) {
  if (message?.engagementTrackingState === 'active') return `Opens ${+message.openCount || 0} · clicks ${+message.clickCount || 0} · Active`
  return message?.engagementTrackingState === 'expired' ? 'Tracking expired; data removed' : 'Tracking off'
}

export async function listEmailMessages({ entityType, entityId, limit, signal } = {}) {
  const params = new URLSearchParams()
  if (entityType) params.set('entityType', entityType)
  if (entityId) params.set('entityId', String(entityId))
  if (limit) params.set('limit', String(limit))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/email-messages${suffix}`, { fallbackMessage: 'Unable to load email log.', signal })

  return payload?.data?.messages || []
}

export async function listMyEmailMessages({ limit, signal } = {}) {
  const params = new URLSearchParams()
  if (limit) params.set('limit', String(limit))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/me/email-messages${suffix}`, { fallbackMessage: 'Unable to load your mailbox.', signal })

  return payload?.data?.messages || []
}

export async function listSharedInboxEmailMessages({ limit, signal } = {}) {
  const params = new URLSearchParams()
  if (limit) params.set('limit', String(limit))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/shared-inbox/email-messages${suffix}`, { fallbackMessage: 'Unable to load shared inbox.', signal })

  return payload?.data?.messages || []
}

export async function getEmailMessage(messageId, { signal } = {}) {
  const payload = await apiRequest(`/api/email-messages/${messageId}`, { fallbackMessage: 'Unable to load email message.', signal })

  return payload?.data?.message
}

export async function updateSharedInboxEmailMessage(messageId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-messages/${messageId}/shared-inbox`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update shared inbox message.', signal })

  return payload?.data?.message
}

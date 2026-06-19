import { apiRequest } from './api'

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

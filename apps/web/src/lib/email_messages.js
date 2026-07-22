import { apiRequest } from './api'

async function listRequest(path, params, fallbackMessage, signal) {
  const query = params.toString()
  return apiRequest(`${path}${query ? `?${query}` : ''}`, { fallbackMessage, signal })
}

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
  const payload = await listRequest('/api/email-messages', params, 'Unable to load email log.', signal)

  return payload?.data?.messages || []
}

export async function listMyEmailMessages({ limit, signal } = {}) {
  const params = new URLSearchParams()
  if (limit) params.set('limit', String(limit))
  const payload = await listRequest('/api/me/email-messages', params, 'Unable to load your mailbox.', signal)

  return payload?.data?.messages || []
}

export async function listSharedInboxEmailMessages({ cursor = '', limit, signal } = {}) {
  const params = new URLSearchParams()
  if (limit) params.set('limit', String(limit))
  if (cursor) params.set('cursor', cursor)
  const payload = await listRequest('/api/shared-inbox/email-messages', params, 'Unable to load shared inbox.', signal)

  const page = payload?.data || {}
  return { messages: page.messages || [], meta: page.meta || { hasMore: false } }
}

export async function getEmailMessage(messageId, { signal } = {}) {
  const payload = await apiRequest(`/api/email-messages/${messageId}`, { fallbackMessage: 'Unable to load email message.', signal })

  return payload?.data?.message
}

export async function getEmailThread(messageId, { signal } = {}) {
  const payload = await apiRequest(`/api/email-threads/${messageId}`, { fallbackMessage: 'Unable to load email thread.', signal })

  return { messages: payload?.data?.messages || [], replies: payload?.data?.replies || [] }
}

export async function sendEmailReply(messageId, body, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest(`/api/email-threads/${messageId}/reply`, {
    method: 'POST', body: { body }, headers: { 'Idempotency-Key': idempotencyKey }, fallbackMessage: 'Unable to send email reply.', signal
  })

  return payload?.data?.reply
}

export async function listRecordEmailDeliveries({ entityType, entityId }, { signal } = {}) {
  const params = new URLSearchParams({ entityType, entityId: String(entityId) })
  const payload = await apiRequest(`/api/record-email-deliveries?${params.toString()}`, { fallbackMessage: 'Unable to load record email delivery status.', signal })
  return payload?.data?.deliveries || []
}

export async function resolveRecordEmailDelivery(deliveryId, resolution, { signal } = {}) {
  const payload = await apiRequest(`/api/record-email-deliveries/${deliveryId}/resolve`, {
    method: 'POST', body: { resolution }, fallbackMessage: 'Unable to resolve record email delivery.', signal
  })
  return payload?.data
}

function recordEmailActionPath(entityType, entityId, action) {
  const route = { contact: 'contacts', company: 'companies', deal: 'deals' }[entityType]
  if (!route || !entityId) throw new Error('A supported record is required for email composition.')
  return `/api/${route}/${entityId}/${action}`
}

export async function previewRecordEmail(entityType, entityId, input, { signal } = {}) {
  const payload = await apiRequest(recordEmailActionPath(entityType, entityId, 'email-preview'), {
    method: 'POST', body: input, fallbackMessage: 'Unable to preview the merged email.', signal
  })
  return payload?.data
}

export async function sendRecordEmailTest(entityType, entityId, input, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest(recordEmailActionPath(entityType, entityId, 'email-test'), {
    method: 'POST', body: input, headers: { 'Idempotency-Key': idempotencyKey }, fallbackMessage: 'Unable to send the template test.', signal
  })
  return payload?.data
}

export async function sendRecordEmail(entityType, entityId, input, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest(recordEmailActionPath(entityType, entityId, 'email'), {
    method: 'POST', body: input, headers: { 'Idempotency-Key': idempotencyKey }, fallbackMessage: 'Unable to send email.', signal
  })
  return payload?.data
}

export async function resolveEmailReply(replyId, resolution, { signal } = {}) {
  const payload = await apiRequest(`/api/email-replies/${replyId}/resolve`, {
    method: 'POST', body: { resolution }, fallbackMessage: 'Unable to resolve email reply.', signal
  })

  return payload?.data?.reply
}

export async function updateSharedInboxEmailMessage(messageId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-messages/${messageId}/shared-inbox`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update shared inbox message.', signal })

  return payload?.data?.message
}

import { apiRequest } from './api'

export async function listSMSMessages({ entityType, entityId, limit, signal } = {}) {
  const params = new URLSearchParams()
  if (entityType) params.set('entityType', entityType)
  if (entityId) params.set('entityId', String(entityId))
  if (limit) params.set('limit', String(limit))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/sms-messages${suffix}`, { fallbackMessage: 'Unable to load SMS history.', signal })

  return payload?.data?.messages || []
}

export async function sendContactSMS(contactId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/contacts/${contactId}/sms`, { method: 'POST', body: input, fallbackMessage: 'Unable to send SMS.', signal })

  return payload?.data?.message
}

export async function logInboundSMS(input, { signal } = {}) {
  const payload = await apiRequest('/api/sms-messages/log', { method: 'POST', body: input, fallbackMessage: 'Unable to log inbound SMS.', signal })

  return payload?.data?.message
}

export async function optOutSMS(input, { signal } = {}) {
  const payload = await apiRequest('/api/sms/opt-outs', { method: 'POST', body: input, fallbackMessage: 'Unable to opt out phone number.', signal })

  return payload?.data?.suppression
}

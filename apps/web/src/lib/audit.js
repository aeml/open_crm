import { apiRequest } from './api'

export async function listAuditEvents({ eventType = '', limit = 50, signal } = {}) {
  const params = new URLSearchParams()
  if (eventType) {
    params.set('eventType', eventType)
  }
  if (limit) {
    params.set('limit', String(limit))
  }

  const query = params.toString()
  const payload = await apiRequest(`/api/audit-events${query ? `?${query}` : ''}`, { fallbackMessage: 'Unable to load audit events.', signal })

  return payload?.data?.events || []
}

import { apiRequest } from './api'

export async function listCalendarEvents({ entityType, entityId, limit, signal } = {}) {
  const params = new URLSearchParams()
  if (entityType) params.set('entityType', entityType)
  if (entityId) params.set('entityId', String(entityId))
  if (limit) params.set('limit', String(limit))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/calendar-events${suffix}`, { fallbackMessage: 'Unable to load meetings.', signal })

  return payload?.data?.events || []
}

export async function scheduleCalendarEvent(input, { signal } = {}) {
  const payload = await apiRequest('/api/calendar-events', { method: 'POST', body: input, fallbackMessage: 'Unable to schedule meeting.', signal })

  return payload?.data?.event
}

export async function cancelCalendarEvent(eventId, { signal } = {}) {
  const payload = await apiRequest(`/api/calendar-events/${eventId}/cancel`, { method: 'PATCH', fallbackMessage: 'Unable to cancel meeting.', signal })

  return payload?.data?.event
}

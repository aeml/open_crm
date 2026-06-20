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

export async function listCalendarAvailability({ signal } = {}) {
  const payload = await apiRequest('/api/me/calendar-availability', { fallbackMessage: 'Unable to load calendar availability.', signal })

  return payload?.data?.blocks || []
}

export async function updateCalendarAvailability(input, { signal } = {}) {
  const payload = await apiRequest('/api/me/calendar-availability', { method: 'PUT', body: input, fallbackMessage: 'Unable to update calendar availability.', signal })

  return payload?.data?.blocks || []
}

export async function listCalendarBookingLinks({ signal } = {}) {
  const payload = await apiRequest('/api/calendar-booking-links', { fallbackMessage: 'Unable to load booking links.', signal })

  return payload?.data?.links || []
}

export async function createCalendarBookingLink(input, { signal } = {}) {
  const payload = await apiRequest('/api/calendar-booking-links', { method: 'POST', body: input, fallbackMessage: 'Unable to save booking link.', signal })

  return payload?.data?.link
}

export async function updateCalendarBookingLink(linkId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/calendar-booking-links/${linkId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update booking link.', signal })

  return payload?.data?.link
}

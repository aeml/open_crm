import { apiRequest } from './api'

export async function listActivities(entityType, entityId, { cursor = '', limit = 50, signal } = {}) {
  const params = new URLSearchParams({ entityType, entityId: String(entityId), limit: String(limit) })
  if (cursor) params.set('cursor', cursor)
  const payload = await apiRequest(`/api/activities?${params.toString()}`, { fallbackMessage: 'Unable to load older activity.', signal })

  return payload?.data || { activities: [], meta: { limit, hasMore: false, nextCursor: '' } }
}

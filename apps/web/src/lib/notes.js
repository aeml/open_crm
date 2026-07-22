import { apiRequest } from './api'

export async function listNotes(entityType, entityId, { cursor = '', limit, signal } = {}) {
  const params = new URLSearchParams({ entityType, entityId: String(entityId) })
  if (limit) params.set('limit', String(limit))
  if (cursor) params.set('cursor', cursor)
  const payload = await apiRequest(`/api/notes?${params.toString()}`, { fallbackMessage: 'Unable to load notes.', signal })

  return payload?.data || { notes: [], meta: { limit: limit || 50, hasMore: false, nextCursor: '' } }
}

export async function createNote(input, { signal } = {}) {
  const payload = await apiRequest('/api/notes', { method: 'POST', body: input, fallbackMessage: 'Unable to create note.', signal })

  return payload?.data
}

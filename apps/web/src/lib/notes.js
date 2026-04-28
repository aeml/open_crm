import { apiRequest } from './api'

export async function listNotes(entityType, entityId, { signal } = {}) {
  const payload = await apiRequest(`/api/notes?entityType=${encodeURIComponent(entityType)}&entityId=${encodeURIComponent(entityId)}`, { fallbackMessage: 'Unable to load notes.', signal })

  return payload?.data?.notes || []
}

export async function createNote(input, { signal } = {}) {
  const payload = await apiRequest('/api/notes', { method: 'POST', body: input, fallbackMessage: 'Unable to create note.', signal })

  return payload?.data
}

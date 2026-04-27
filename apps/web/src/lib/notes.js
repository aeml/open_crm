import { apiRequest } from './api'

export async function listNotes(entityType, entityId) {
  const payload = await apiRequest(`/api/notes?entityType=${encodeURIComponent(entityType)}&entityId=${encodeURIComponent(entityId)}`, { fallbackMessage: 'Unable to load notes.' })

  return payload?.data?.notes || []
}

export async function createNote(input) {
  const payload = await apiRequest('/api/notes', { method: 'POST', body: input, fallbackMessage: 'Unable to create note.' })

  return payload?.data
}

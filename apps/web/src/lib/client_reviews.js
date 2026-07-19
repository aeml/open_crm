import { apiRequest } from './api'

export async function getClientReview(entityType, entityId, { signal } = {}) {
  const payload = await apiRequest(`/api/client-reviews/${entityType}/${entityId}`, { fallbackMessage: 'Unable to load client review schedule.', signal })
  return payload?.data
}

export async function upsertClientReview(entityType, entityId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/client-reviews/${entityType}/${entityId}`, {
    method: 'PUT',
    body: input,
    fallbackMessage: 'Unable to save client review schedule.',
    signal
  })
  return payload?.data
}

export async function deleteClientReview(entityType, entityId, { signal } = {}) {
  await apiRequest(`/api/client-reviews/${entityType}/${entityId}`, { method: 'DELETE', fallbackMessage: 'Unable to clear client review schedule.', signal })
}

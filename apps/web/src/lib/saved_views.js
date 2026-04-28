import { apiRequest } from './api'

export async function listSavedViews(entityType, { signal } = {}) {
  const params = new URLSearchParams({ entityType })
  const payload = await apiRequest(`/api/saved-views?${params.toString()}`, { fallbackMessage: 'Unable to load saved views.', signal })

  return payload?.data?.views || []
}

export async function createSavedView(input, { signal } = {}) {
  const payload = await apiRequest('/api/saved-views', { method: 'POST', body: input, fallbackMessage: 'Unable to save view.', signal })

  return payload?.data?.view
}

export async function updateSavedView(viewID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/saved-views/${viewID}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update saved view.', signal })

  return payload?.data?.view
}

export async function deleteSavedView(viewID, { signal } = {}) {
  await apiRequest(`/api/saved-views/${viewID}`, { method: 'DELETE', fallbackMessage: 'Unable to delete saved view.', signal })
}

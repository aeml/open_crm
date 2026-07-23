import { apiRequest } from './api'
import { loadCompleteCatalog } from './complete_catalog'

export async function listSavedViewPage(entityType, { page = 1, pageSize = 50, signal } = {}) {
  const params = new URLSearchParams({ entityType, page: String(page), pageSize: String(pageSize) })
  const payload = await apiRequest(`/api/saved-views?${params.toString()}`, { fallbackMessage: 'Unable to load saved views.', signal })
  const data = payload?.data || {}
  const views = data.views || []

  return { views, meta: data.meta || { page, pageSize, total: views.length } }
}

export async function listSavedViews(entityType, { signal } = {}) {
  return loadCompleteCatalog(
    ({ page, pageSize }) => listSavedViewPage(entityType, { page, pageSize, signal }),
    'views',
    'The saved-view catalog changed while options were loading. Try again.',
    'The complete saved-view catalog could not be loaded. Delete legacy overflow and try again.'
  )
}

export async function createSavedView(input, { signal } = {}) {
  const payload = await apiRequest('/api/saved-views', { method: 'POST', body: input, fallbackMessage: 'Unable to save view.', signal })

  return payload?.data?.view
}

export async function updateSavedView(viewID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/saved-views/${viewID}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update saved view.', signal })

  return payload?.data?.view
}

export async function deleteSavedView(viewID, revision, { signal } = {}) {
  await apiRequest(`/api/saved-views/${viewID}?revision=${encodeURIComponent(revision)}`, { method: 'DELETE', fallbackMessage: 'Unable to delete saved view.', signal })
}

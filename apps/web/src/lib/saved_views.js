import { apiRequest } from './api'

export async function listSavedViewPage(entityType, { page = 1, pageSize = 50, signal } = {}) {
  const params = new URLSearchParams({ entityType, page: String(page), pageSize: String(pageSize) })
  const payload = await apiRequest(`/api/saved-views?${params.toString()}`, { fallbackMessage: 'Unable to load saved views.', signal })
  const data = payload?.data || {}
  const views = data.views || []

  return { views, meta: data.meta || { page, pageSize, total: views.length } }
}

export async function listSavedViews(entityType, { signal } = {}) {
  const viewsById = new Map()
  let expectedTotal = null
  for (let page = 1; page <= 501; page += 1) {
    const result = await listSavedViewPage(entityType, { page, pageSize: 100, signal })
    const total = Number(result.meta?.total)
    if (!Number.isSafeInteger(total) || total < 0 || (expectedTotal !== null && total !== expectedTotal)) {
      throw new Error('The saved-view catalog changed while options were loading. Try again.')
    }
    expectedTotal = total
    result.views.forEach((view) => viewsById.set(view.id, view))
    if (viewsById.size >= expectedTotal) return [...viewsById.values()]
    if (result.views.length === 0) break
  }

  throw new Error('The complete saved-view catalog could not be loaded. Delete legacy overflow and try again.')
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

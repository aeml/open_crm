import { apiRequest } from './api'

export async function listArchivedRecords({ entityType = '', search = '', limit = 50, signal } = {}) {
  const params = new URLSearchParams({ limit: String(limit) })
  if (entityType) params.set('entityType', entityType)
  if (search) params.set('q', search)
  const payload = await apiRequest(`/api/data-operations/archive?${params.toString()}`, {
    fallbackMessage: 'Unable to load archived records.',
    signal
  })
  return payload?.data?.records || []
}

export async function restoreArchivedRecord(entityType, entityId) {
  const payload = await apiRequest(`/api/data-operations/archive/${encodeURIComponent(entityType)}/${entityId}/restore`, {
    method: 'POST',
    fallbackMessage: 'Unable to restore archived record.'
  })
  return payload?.data?.record || null
}

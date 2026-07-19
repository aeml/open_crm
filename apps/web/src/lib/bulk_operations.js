import { apiRequest } from './api'

export async function executeBulkOperation(input) {
  const payload = await apiRequest('/api/data-operations/bulk', {
    method: 'POST',
    body: input,
    fallbackMessage: 'Unable to apply bulk change.'
  })
  return payload?.data?.operation || null
}

export async function listBulkOperations({ entityType = '', limit = 5, signal } = {}) {
  const params = new URLSearchParams({ limit: String(limit) })
  if (entityType) params.set('entityType', entityType)
  const payload = await apiRequest(`/api/data-operations/bulk?${params.toString()}`, {
    fallbackMessage: 'Unable to load recent bulk changes.',
    signal
  })
  return payload?.data?.operations || []
}

export async function rollbackBulkOperation(operationId) {
  const payload = await apiRequest(`/api/data-operations/bulk/${operationId}/rollback`, {
    method: 'POST',
    fallbackMessage: 'Unable to undo bulk change.'
  })
  return payload?.data?.operation || null
}

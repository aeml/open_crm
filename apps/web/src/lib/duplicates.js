import { apiRequest } from './api'

export async function reviewDuplicates({ entityType = 'contact', limit = 20, signal } = {}) {
  const params = new URLSearchParams({ entityType, limit: String(limit) })
  const payload = await apiRequest(`/api/data-operations/duplicates?${params.toString()}`, {
    fallbackMessage: 'Unable to review possible duplicates.',
    signal
  })
  return payload?.data || { candidates: [], recentMerges: [] }
}

export async function mergeDuplicate(input) {
  const payload = await apiRequest('/api/data-operations/duplicates/merge', {
    method: 'POST',
    body: input,
    fallbackMessage: 'Unable to merge duplicate records.'
  })
  return payload?.data?.operation || null
}

import { apiRequest } from './api'

export async function getTouchpointSummary(entityType, entityId, { staleDays = 30, signal } = {}) {
  const params = new URLSearchParams({ staleDays: String(staleDays) })
  const payload = await apiRequest(`/api/touchpoints/${entityType}/${entityId}?${params.toString()}`, { fallbackMessage: 'Unable to load follow-up history.', signal })
  const data = payload?.data || {}
  return { ...data, recent: Array.isArray(data.recent) ? data.recent : [], semantics: Array.isArray(data.semantics) ? data.semantics : [], healthReasons: Array.isArray(data.healthReasons) ? data.healthReasons : [], healthSemantics: Array.isArray(data.healthSemantics) ? data.healthSemantics : [] }
}

export async function getClientHealthReport({ entityType = 'company', status = 'all', staleDays = 30, ownerUserId = 0, limit = 25, signal } = {}) {
  const params = new URLSearchParams({ entityType, status, staleDays: String(staleDays), limit: String(limit) })
  if (ownerUserId) params.set('ownerUserId', String(ownerUserId))
  const payload = await apiRequest(`/api/reports/client-health?${params.toString()}`, { fallbackMessage: 'Unable to load client health.', signal })
  const data = payload?.data || {}
  return { ...data, totals: data.totals || { total: 0, healthy: 0, watch: 0, needsAttention: 0 }, records: Array.isArray(data.records) ? data.records : [], semantics: Array.isArray(data.semantics) ? data.semantics : [] }
}

export async function getFollowUpReport({ entityType = 'contact', staleDays = 30, ownerUserId = 0, limit = 25, signal } = {}) {
  const params = new URLSearchParams({ entityType, staleDays: String(staleDays), limit: String(limit) })
  if (ownerUserId) params.set('ownerUserId', String(ownerUserId))
  const payload = await apiRequest(`/api/reports/follow-up?${params.toString()}`, { fallbackMessage: 'Unable to load the follow-up queue.', signal })
  const data = payload?.data || {}
  return { ...data, records: Array.isArray(data.records) ? data.records : [], semantics: Array.isArray(data.semantics) ? data.semantics : [] }
}

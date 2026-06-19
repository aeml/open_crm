import { apiRequest } from './api'

export async function listCalls({ entityType, entityId, limit, signal } = {}) {
  const params = new URLSearchParams()
  if (entityType) params.set('entityType', entityType)
  if (entityId) params.set('entityId', String(entityId))
  if (limit) params.set('limit', String(limit))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/calls${suffix}`, { fallbackMessage: 'Unable to load call history.', signal })

  return payload?.data?.calls || []
}

export async function startCall(input, { signal } = {}) {
  const payload = await apiRequest('/api/calls/start', { method: 'POST', body: input, fallbackMessage: 'Unable to start call.', signal })

  return payload?.data
}

export async function completeCall(callId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/calls/${callId}/complete`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to log call outcome.', signal })

  return payload?.data?.call
}

export async function logCall(input, { signal } = {}) {
  const payload = await apiRequest('/api/calls/log', { method: 'POST', body: input, fallbackMessage: 'Unable to log call.', signal })

  return payload?.data?.call
}

export async function updateCallRecording(callId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/calls/${callId}/recording`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update call recording.', signal })

  return payload?.data?.call
}

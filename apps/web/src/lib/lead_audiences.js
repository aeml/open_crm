import { apiRequest } from './api'

export async function listLeadAudiences({ signal } = {}) {
  const payload = await apiRequest('/api/lead-audiences', { fallbackMessage: 'Unable to load lead audiences.', signal })

  return payload?.data?.audiences || []
}

export async function createLeadAudience(input, { signal } = {}) {
  const payload = await apiRequest('/api/lead-audiences', { method: 'POST', body: input, fallbackMessage: 'Unable to save lead audience.', signal })

  return payload?.data?.audience
}

export async function updateLeadAudience(audienceId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/lead-audiences/${audienceId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update lead audience.', signal })

  return payload?.data?.audience
}

export async function previewLeadAudience(filters, { signal } = {}) {
  const payload = await apiRequest('/api/lead-audiences/preview', { method: 'POST', body: { filters }, fallbackMessage: 'Unable to preview lead audience.', signal })

  return payload?.data
}

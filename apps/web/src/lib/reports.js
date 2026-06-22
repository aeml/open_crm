import { apiRequest } from './api'

export async function listReportDefinitions({ signal } = {}) {
  const payload = await apiRequest('/api/report-definitions', { fallbackMessage: 'Unable to load report definitions.', signal })

  return payload?.data?.definitions || []
}

export async function createReportDefinition(input, { signal } = {}) {
  const payload = await apiRequest('/api/report-definitions', { method: 'POST', body: input, fallbackMessage: 'Unable to save report definition.', signal })

  return payload?.data?.definition
}

export async function updateReportDefinition(definitionId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/report-definitions/${definitionId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update report definition.', signal })

  return payload?.data?.definition
}

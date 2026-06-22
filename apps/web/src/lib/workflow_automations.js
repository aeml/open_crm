import { apiRequest } from './api'

export async function listWorkflowAutomations({ signal } = {}) {
  const payload = await apiRequest('/api/workflow-automations', { fallbackMessage: 'Unable to load workflow automations.', signal })

  return payload?.data?.automations || []
}

export async function listWorkflowAutomationRuns({ automationId, limit = 10, signal } = {}) {
  const params = new URLSearchParams()
  if (automationId) params.set('automationId', String(automationId))
  if (limit) params.set('limit', String(limit))
  const query = params.toString()
  const payload = await apiRequest(`/api/workflow-automation-runs${query ? `?${query}` : ''}`, { fallbackMessage: 'Unable to load workflow automation runs.', signal })

  return payload?.data?.runs || []
}

export async function createWorkflowAutomation(input, { signal } = {}) {
  const payload = await apiRequest('/api/workflow-automations', { method: 'POST', body: input, fallbackMessage: 'Unable to save workflow automation.', signal })

  return payload?.data?.automation
}

export async function updateWorkflowAutomation(automationId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/workflow-automations/${automationId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update workflow automation.', signal })

  return payload?.data?.automation
}

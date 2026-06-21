import { apiRequest } from './api'

export async function listWorkflowAutomations({ signal } = {}) {
  const payload = await apiRequest('/api/workflow-automations', { fallbackMessage: 'Unable to load workflow automations.', signal })

  return payload?.data?.automations || []
}

export async function createWorkflowAutomation(input, { signal } = {}) {
  const payload = await apiRequest('/api/workflow-automations', { method: 'POST', body: input, fallbackMessage: 'Unable to save workflow automation.', signal })

  return payload?.data?.automation
}

export async function updateWorkflowAutomation(automationId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/workflow-automations/${automationId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update workflow automation.', signal })

  return payload?.data?.automation
}

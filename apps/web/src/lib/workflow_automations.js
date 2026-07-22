import { apiRequest } from './api'

export async function listWorkflowAutomations({ page = 1, pageSize = 50, signal } = {}) {
  const payload = await apiRequest(`/api/workflow-automations?page=${page}&pageSize=${pageSize}`, { fallbackMessage: 'Unable to load workflow automations.', signal })
  const automations = Array.isArray(payload?.data?.automations) ? payload.data.automations : []
  const responseMeta = payload?.data?.meta
  if (responseMeta && (!Number.isInteger(responseMeta.page) || responseMeta.page !== page || !Number.isInteger(responseMeta.pageSize) || responseMeta.pageSize !== pageSize || automations.length > responseMeta.pageSize || !Number.isInteger(responseMeta.total) || responseMeta.total < automations.length || !Number.isInteger(responseMeta.activeActionCount) || responseMeta.activeActionCount < 0)) {
    throw new Error('The server returned an invalid workflow automation page. Refresh before retrying.')
  }

  return {
    automations,
    meta: responseMeta || {
      page,
      pageSize,
      total: automations.length,
      activeActionCount: automations.reduce((total, automation) => total + (automation.isActive ? (automation.actions || []).length : 0), 0)
    }
  }
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

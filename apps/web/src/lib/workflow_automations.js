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

  const runs = Array.isArray(payload?.data?.runs) ? payload.data.runs : []
  return runs.map((run) => {
    const actions = Array.isArray(run?.actions) ? run.actions : []
    if (!validRunCausality(run) || actions.some((action) => !validRunAction(action))) {
      throw new Error('The server returned invalid workflow action evidence. Refresh before retrying.')
    }
    return { ...run, causalDepth: Number.isInteger(run.causalDepth) ? run.causalDepth : 0, actions }
  })
}

function validRunCausality(run) {
  const depth = Number.isInteger(run?.causalDepth) ? run.causalDepth : 0
  const causeRunID = Number(run?.causationRunId || 0)
  const causeAction = Number(run?.causationActionPosition || 0)
  return depth >= 0 && depth <= 9 && (
    (depth === 0 && causeRunID === 0 && causeAction === 0) ||
    (depth > 0 && Number.isInteger(causeRunID) && causeRunID > 0 && Number.isInteger(causeAction) && causeAction > 0 && causeAction <= 25)
  )
}

export async function listWorkflowApprovals({ signal } = {}) {
	const payload = await apiRequest('/api/workflow-approvals', { fallbackMessage: 'Unable to load workflow approvals.', signal })
	const approvals = Array.isArray(payload?.data?.approvals) ? payload.data.approvals : []
	if (approvals.some((approval) => !validWorkflowApproval(approval))) {
		throw new Error('The server returned invalid workflow approval evidence. Refresh before retrying.')
	}
	return approvals
}

function validWorkflowApproval(approval) {
	return Number.isInteger(approval?.id) && approval.id > 0 &&
		Number.isInteger(approval.runId) && approval.runId > 0 &&
		Number.isInteger(approval.automationId) && approval.automationId > 0 &&
		Number.isInteger(approval.dealId) && approval.dealId > 0 &&
		typeof approval.name === 'string' && approval.name.length > 0 &&
		['owner', 'admin', 'record_owner'].includes(approval.approverRole) &&
		approval.status === 'pending' &&
		Number.isInteger(approval.pendingTaskCount) && approval.pendingTaskCount >= 1 && approval.pendingTaskCount <= 5
}

export async function decideWorkflowApproval(approvalId, input, idempotencyKey, { signal } = {}) {
	const payload = await apiRequest(`/api/workflow-approvals/${approvalId}/decision`, {
		method: 'POST',
		body: input,
		headers: { 'Idempotency-Key': idempotencyKey },
		fallbackMessage: 'Unable to decide workflow approval.',
		signal
	})
	return payload?.data?.approval
}

const runActionStatuses = new Set(['queued', 'running', 'succeeded', 'failed', 'skipped', 'cancelled'])

function validRunAction(action) {
	const assignmentSucceeded = action?.type === 'assign_owner' && action?.status === 'succeeded'
	const sequenceSucceeded = action?.type === 'add_to_sequence' && action?.status === 'succeeded'
  return Number.isInteger(action?.id) && action.id > 0 &&
    Number.isInteger(action.position) && action.position > 0 &&
    typeof action.type === 'string' && action.type.length > 0 &&
    typeof action.label === 'string' && action.label.length > 0 &&
    runActionStatuses.has(action.status) &&
    Number.isInteger(action.attempts) && action.attempts >= 0 &&
    typeof action.scheduledAt === 'string' && action.scheduledAt.length > 0 &&
    (!action.taskId || (Number.isInteger(action.taskId) && action.taskId > 0)) &&
    (!action.notificationCount || (action.type === 'notify' && Number.isInteger(action.notificationCount) && action.notificationCount > 0 && action.notificationCount <= 50)) &&
		(!action.assignedUserId || (action.type === 'assign_owner' && Number.isInteger(action.assignedUserId) && action.assignedUserId > 0)) &&
		(!assignmentSucceeded || (Number.isInteger(action.assignedUserId) && action.assignedUserId > 0 && typeof action.assignedUserName === 'string' && action.assignedUserName.length > 0 && typeof action.assignmentChanged === 'boolean')) &&
		(!action.assignmentChanged || assignmentSucceeded) &&
		(!action.sequenceEnrollmentId || (sequenceSucceeded && Number.isInteger(action.sequenceId) && action.sequenceId > 0 && typeof action.sequenceName === 'string' && action.sequenceName.length > 0 && Number.isInteger(action.sequenceEnrollmentId) && action.sequenceEnrollmentId > 0 && Number.isInteger(action.sequenceContactId) && action.sequenceContactId > 0 && typeof action.sequenceContactName === 'string' && action.sequenceContactName.length > 0 && typeof action.sequenceEnrollmentCreated === 'boolean')) &&
		(!sequenceSucceeded || (Number.isInteger(action.sequenceId) && action.sequenceId > 0 && Number.isInteger(action.sequenceEnrollmentId) && action.sequenceEnrollmentId > 0 && Number.isInteger(action.sequenceContactId) && action.sequenceContactId > 0)) &&
		(!action.sequenceEnrollmentCreated || sequenceSucceeded) &&
    (!action.approval || validRunActionApproval(action.approval))
}

function validRunActionApproval(approval) {
  return Number.isInteger(approval?.id) && approval.id > 0 &&
    ['pending', 'approved', 'rejected', 'cancelled'].includes(approval.status) &&
    ['owner', 'admin', 'record_owner'].includes(approval.approverRole) &&
    typeof approval.message === 'string' && approval.message.length > 0 &&
    Number.isInteger(approval.requestedByUserId) && approval.requestedByUserId > 0 &&
    typeof approval.requestedAt === 'string' && approval.requestedAt.length > 0
}

export async function createWorkflowAutomation(input, { signal } = {}) {
  const payload = await apiRequest('/api/workflow-automations', { method: 'POST', body: input, fallbackMessage: 'Unable to save workflow automation.', signal })

  return payload?.data?.automation
}

export async function updateWorkflowAutomation(automationId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/workflow-automations/${automationId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update workflow automation.', signal })

  return payload?.data?.automation
}

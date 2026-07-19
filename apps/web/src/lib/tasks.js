import { apiRequest, apiURL } from './api'

export async function listTasks(query = {}, { signal } = {}) {
  const params = new URLSearchParams()
  if (query.status) params.set('status', query.status)
  if (query.entityType) params.set('entityType', query.entityType)
  if (query.entityId) params.set('entityId', String(query.entityId))
  if (query.due && query.due !== 'all') params.set('due', query.due)
  if (query.unassigned) params.set('unassigned', 'true')
  else if (query.assignedToUserId) params.set('assignedToUserId', String(query.assignedToUserId))
  if (query.search) params.set('q', query.search)
  const suffix = params.toString() ? `?${params.toString()}` : ''

  const payload = await apiRequest(`/api/tasks${suffix}`, { fallbackMessage: 'Unable to load tasks.', signal })

  return payload?.data || { tasks: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 } }
}

export async function createTask(input, { signal } = {}) {
  const payload = await apiRequest('/api/tasks', { method: 'POST', body: input, fallbackMessage: 'Unable to create task.', signal })

  return payload?.data
}

export async function getTask(taskID, { signal } = {}) {
  const payload = await apiRequest(`/api/tasks/${taskID}`, { fallbackMessage: 'Unable to load task.', signal })

  return payload?.data
}

export async function updateTask(taskID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/tasks/${taskID}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update task.', signal })

  return payload?.data
}

export async function archiveTask(taskID, { signal } = {}) {
  await apiRequest(`/api/tasks/${taskID}`, { method: 'DELETE', fallbackMessage: 'Unable to archive task.', signal })
}

export function tasksExportURL(query = {}) {
  const params = new URLSearchParams()
  if (query.status) params.set('status', query.status)
  if (query.due) params.set('due', query.due)
  if (query.assignee) params.set('assignee', query.assignee)
  if (query.entityType) params.set('entityType', query.entityType)
  if (query.entityId) params.set('entityId', String(query.entityId))
  if (query.search) params.set('q', query.search)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return apiURL(`/api/export/tasks${suffix}`)
}

import { apiRequest } from './api'

export async function listTasks(query = {}) {
  const params = new URLSearchParams()
  if (query.status) params.set('status', query.status)
  if (query.entityType) params.set('entityType', query.entityType)
  if (query.entityId) params.set('entityId', String(query.entityId))
  if (query.search) params.set('q', query.search)
  const suffix = params.toString() ? `?${params.toString()}` : ''

  const payload = await apiRequest(`/api/tasks${suffix}`, { fallbackMessage: 'Unable to load tasks.' })

  return payload?.data || { tasks: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 } }
}

export async function createTask(input) {
  const payload = await apiRequest('/api/tasks', { method: 'POST', body: input, fallbackMessage: 'Unable to create task.' })

  return payload?.data
}

export async function getTask(taskID) {
  const payload = await apiRequest(`/api/tasks/${taskID}`, { fallbackMessage: 'Unable to load task.' })

  return payload?.data
}

export async function updateTask(taskID, input) {
  const payload = await apiRequest(`/api/tasks/${taskID}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update task.' })

  return payload?.data
}

export async function archiveTask(taskID) {
  await apiRequest(`/api/tasks/${taskID}`, { method: 'DELETE', fallbackMessage: 'Unable to archive task.' })
}

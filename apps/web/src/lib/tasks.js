import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

async function readJSON(response) {
  if (response.status === 204) {
    return {}
  }
  return response.json()
}

export async function listTasks(query = {}) {
  const params = new URLSearchParams()
  if (query.status) params.set('status', query.status)
  if (query.entityType) params.set('entityType', query.entityType)
  if (query.entityId) params.set('entityId', String(query.entityId))
  if (query.search) params.set('q', query.search)
  const suffix = params.toString() ? `?${params.toString()}` : ''

  const response = await fetch(`${API_BASE_URL}/api/tasks${suffix}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load tasks.'))
  }

  return payload?.data || { tasks: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 } }
}

export async function createTask(input) {
  const response = await fetch(`${API_BASE_URL}/api/tasks`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to create task.'))
  }

  return payload?.data
}

export async function getTask(taskID) {
  const response = await fetch(`${API_BASE_URL}/api/tasks/${taskID}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load task.'))
  }

  return payload?.data
}

export async function updateTask(taskID, input) {
  const response = await fetch(`${API_BASE_URL}/api/tasks/${taskID}`, {
    method: 'PATCH',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to update task.'))
  }

  return payload?.data
}

export async function archiveTask(taskID) {
  const response = await fetch(`${API_BASE_URL}/api/tasks/${taskID}`, {
    method: 'DELETE',
    credentials: 'include'
  })

  if (!response.ok) {
    const payload = await readJSON(response)
    throw new Error(getErrorMessage(payload, 'Unable to archive task.'))
  }
}

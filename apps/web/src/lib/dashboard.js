import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

async function readJSON(response) {
  if (!response || typeof response.json !== 'function') {
    return {}
  }
  if (response.status === 204) {
    return {}
  }
  return response.json()
}

export async function getDashboardSummary() {
  const response = await fetch(`${API_BASE_URL}/api/dashboard/summary`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load dashboard summary.'))
  }

  return payload?.data || {
    pipelineValue: '0',
    openDealsCount: 0,
    wonDealsCount: 0,
    openTasksCount: 0,
    dueTodayCount: 0,
    newContactsCount: 0,
    recentActivities: []
  }
}

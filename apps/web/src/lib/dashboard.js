import { apiRequest } from './api'

export async function getDashboardSummary() {
  const payload = await apiRequest('/api/dashboard/summary', { fallbackMessage: 'Unable to load dashboard summary.' })

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

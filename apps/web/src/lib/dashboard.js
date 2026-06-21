import { apiRequest } from './api'

export async function getDashboardSummary({ signal } = {}) {
  const payload = await apiRequest('/api/dashboard/summary', { fallbackMessage: 'Unable to load dashboard summary.', signal })

  return payload?.data || {
    pipelineValue: '0',
    openDealsCount: 0,
    wonDealsCount: 0,
    openTasksCount: 0,
    dueTodayCount: 0,
    newContactsCount: 0,
    forecast: {
      periodStart: '',
      periodEnd: '',
      currency: 'USD',
      teamQuota: '0',
      wonAmount: '0',
      openPipelineAmount: '0',
      weightedForecastAmount: '0',
      attainmentPct: '0',
      coveragePct: '0',
      members: []
    },
    recentActivities: []
  }
}

export async function upsertSalesQuota(userID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/dashboard/sales-quotas/${userID}`, { method: 'PUT', body: input, fallbackMessage: 'Unable to save sales quota.', signal })

  return payload?.data
}

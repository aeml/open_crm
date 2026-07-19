import { apiRequest } from './api'

export async function getDashboardSummary({ forecastStart = '', forecastEnd = '', signal } = {}) {
  const params = new URLSearchParams()
  if (forecastStart) params.set('forecastStart', forecastStart)
  if (forecastEnd) params.set('forecastEnd', forecastEnd)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/dashboard/summary${suffix}`, { fallbackMessage: 'Unable to load dashboard summary.', signal })

  return payload?.data || {
    pipelineValue: '0',
    baseCurrency: 'USD',
    missingRateCurrencies: [],
    openDealsCount: 0,
    wonDealsCount: 0,
    openTasksCount: 0,
    overdueTasksCount: 0,
    dueSoonTasksCount: 0,
    upcomingTasksCount: 0,
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
      missingRateCurrencies: [],
      members: [],
      stages: []
    },
    recentActivities: []
  }
}

export async function upsertSalesQuota(userID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/dashboard/sales-quotas/${userID}`, { method: 'PUT', body: input, fallbackMessage: 'Unable to save sales quota.', signal })

  return payload?.data
}

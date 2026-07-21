import { apiRequest, apiURL } from './api'

export async function listReportDefinitions({ signal } = {}) {
  const payload = await apiRequest('/api/report-definitions', { fallbackMessage: 'Unable to load report definitions.', signal })

  return payload?.data?.definitions || []
}

export async function createReportDefinition(input, { signal } = {}) {
  const payload = await apiRequest('/api/report-definitions', { method: 'POST', body: input, fallbackMessage: 'Unable to save report definition.', signal })

  return payload?.data?.definition
}

export async function updateReportDefinition(definitionId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/report-definitions/${definitionId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update report definition.', signal })

  return payload?.data?.definition
}

export async function getReportResults(definitionId, { page = 1, pageSize = 50, signal } = {}) {
  const params = new URLSearchParams({ page: String(page), pageSize: String(pageSize) })
  const payload = await apiRequest(`/api/report-definitions/${definitionId}/results?${params.toString()}`, { fallbackMessage: 'Unable to run saved report.', signal })
  const data = payload?.data || {}
  return {
    ...data,
    columns: Array.isArray(data.columns) ? data.columns : [],
    rows: Array.isArray(data.rows) ? data.rows : [],
    page: data.page || page,
    pageSize: data.pageSize || pageSize,
    hasMore: data.hasMore === true
  }
}

export function reportExportURL(definitionId) {
  return apiURL(`/api/report-definitions/${definitionId}/export.csv`)
}

export async function getDataQualitySummary({ staleDays = 30, signal } = {}) {
  const params = new URLSearchParams({ staleDays: String(staleDays) })
  const payload = await apiRequest(`/api/data-quality/summary?${params.toString()}`, { fallbackMessage: 'Unable to load data quality reports.', signal })
  const data = payload?.data || {}
  return { ...data, reports: Array.isArray(data.reports) ? data.reports : [], staleDays: data.staleDays || staleDays }
}

export async function getSalesActivityReport({ from, to, ownerUserId = 0, signal } = {}) {
  const params = new URLSearchParams()
  if (from) params.set('from', from)
  if (to) params.set('to', to)
  if (ownerUserId) params.set('ownerUserId', String(ownerUserId))
  const payload = await apiRequest(`/api/reports/sales-activity?${params.toString()}`, { fallbackMessage: 'Unable to load sales activity reporting.', signal })
  const data = payload?.data || {}
  return {
    ...data,
    totals: data.totals || {},
    owners: Array.isArray(data.owners) ? data.owners : [],
    stages: Array.isArray(data.stages) ? data.stages : [],
    dealEvents: Array.isArray(data.dealEvents) ? data.dealEvents : []
  }
}

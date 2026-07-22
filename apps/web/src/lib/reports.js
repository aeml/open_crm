import { apiRequest, apiURL } from './api'

export async function listReportDefinitions({ page = 1, pageSize = 50, signal } = {}) {
  const payload = await apiRequest(`/api/report-definitions?page=${page}&pageSize=${pageSize}`, { fallbackMessage: 'Unable to load report definitions.', signal })
  const definitions = Array.isArray(payload?.data?.definitions) ? payload.data.definitions : []
  const responseMeta = payload?.data?.meta
  if (responseMeta && (!Number.isInteger(responseMeta.page) || responseMeta.page !== page || !Number.isInteger(responseMeta.pageSize) || responseMeta.pageSize !== pageSize || definitions.length > responseMeta.pageSize || !Number.isInteger(responseMeta.total) || responseMeta.total < definitions.length)) {
    throw new Error('The server returned an invalid report definition page. Refresh before retrying.')
  }

  return {
    definitions,
    meta: responseMeta || { page, pageSize, total: definitions.length }
  }
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
    visualizationContract: data.visualizationContract || '',
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

export async function getPipelineFunnelReport({ pipelineId, entryStageId, from, to, asOf, ownerUserId = 0, signal } = {}) {
  const params = new URLSearchParams({ pipelineId: String(pipelineId), entryStageId: String(entryStageId), from, to, asOf })
  if (ownerUserId) params.set('ownerUserId', String(ownerUserId))
  const payload = await apiRequest(`/api/reports/pipeline-funnel?${params.toString()}`, { fallbackMessage: 'Unable to load pipeline conversion and velocity.', signal })
  const data = payload?.data || {}
  return {
    ...data,
    totals: data.totals || {},
    stages: Array.isArray(data.stages) ? data.stages : [],
    semantics: Array.isArray(data.semantics) ? data.semantics : []
  }
}

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

const sharedDashboardWidths = new Set(['half', 'full'])

function validDashboardRevision(value) {
  return Number.isInteger(value) && value >= 0
}

function normalizeSharedDashboard(payload) {
  const dashboard = payload?.data?.dashboard
  if (!dashboard || !validDashboardRevision(dashboard.revision) || !Array.isArray(dashboard.widgets) || dashboard.widgets.length > 6) {
    throw new Error('The server returned an invalid shared dashboard. Refresh before retrying.')
  }
  const seen = new Set()
  const widgets = dashboard.widgets.map((widget, position) => {
    const definitionId = widget?.reportDefinitionId
    if (!Number.isInteger(widget?.id) || widget.id <= 0 || !Number.isInteger(definitionId) || definitionId <= 0 || widget.position !== position || !sharedDashboardWidths.has(widget.width) || seen.has(definitionId) || !widget.definition || widget.definition.id !== definitionId || typeof widget.definition.name !== 'string') {
      throw new Error('The server returned an invalid shared dashboard. Refresh before retrying.')
    }
    seen.add(definitionId)
    return widget
  })
  if ((dashboard.id === 0 && dashboard.revision !== 0) || (dashboard.id !== 0 && (!Number.isInteger(dashboard.id) || dashboard.id <= 0))) {
    throw new Error('The server returned an invalid shared dashboard. Refresh before retrying.')
  }
  return { ...dashboard, widgets }
}

function normalizeDashboardInput(input) {
  if (!validDashboardRevision(input?.revision) || !Array.isArray(input?.widgets) || input.widgets.length > 6) {
    throw new Error('Choose no more than six shared dashboard reports.')
  }
  const seen = new Set()
  const widgets = input.widgets.map((widget) => {
    if (!Number.isInteger(widget?.reportDefinitionId) || widget.reportDefinitionId <= 0 || !sharedDashboardWidths.has(widget.width) || seen.has(widget.reportDefinitionId)) {
      throw new Error('Choose distinct grouped-bar reports and a supported dashboard width.')
    }
    seen.add(widget.reportDefinitionId)
    return { reportDefinitionId: widget.reportDefinitionId, width: widget.width }
  })
  return { revision: input.revision, widgets }
}

export async function getSharedReportDashboard({ signal } = {}) {
  const payload = await apiRequest('/api/report-dashboard', { fallbackMessage: 'Unable to load the shared report dashboard.', signal })
  return normalizeSharedDashboard(payload)
}

export async function updateSharedReportDashboard(input, { signal } = {}) {
  const normalizedInput = normalizeDashboardInput(input)
  const payload = await apiRequest('/api/report-dashboard', { method: 'PUT', body: normalizedInput, fallbackMessage: 'Unable to update the shared report dashboard.', signal })
  const dashboard = normalizeSharedDashboard(payload)
  if (dashboard.widgets.length !== normalizedInput.widgets.length || dashboard.widgets.some((widget, index) => widget.reportDefinitionId !== normalizedInput.widgets[index].reportDefinitionId || widget.width !== normalizedInput.widgets[index].width)) {
    throw new Error('The shared dashboard response did not match the saved configuration. Refresh before retrying.')
  }
  return dashboard
}

export async function getSharedReportDashboardResults({ signal } = {}) {
  const payload = await apiRequest('/api/report-dashboard/results', { fallbackMessage: 'Unable to run the shared report dashboard.', signal })
  const data = payload?.data
  if (!data || !validDashboardRevision(data.revision) || !Array.isArray(data.widgets) || data.widgets.length > 6 || Number.isNaN(Date.parse(data.generatedAt))) {
    throw new Error('The server returned invalid shared dashboard results. Refresh before retrying.')
  }
  const widgets = data.widgets.map((widget, position) => {
    const definition = widget?.definition
    const result = widget?.result
    if (widget?.position !== position || !sharedDashboardWidths.has(widget?.width) || !definition || !Number.isInteger(definition.id) || definition.id <= 0 || definition.isActive !== true || definition.visualizationType !== 'bar' || definition.visualizationContract !== 'grouped_bar_v1' || !result || result.definitionId !== definition.id || result.visualizationType !== 'bar' || result.visualizationContract !== 'grouped_bar_v1' || result.page !== 1 || result.pageSize !== 12 || !Array.isArray(result.columns) || result.columns.length !== 2 || !Array.isArray(result.rows) || result.rows.length > 12 || result.generatedAt !== data.generatedAt) {
      throw new Error('The server returned invalid shared dashboard results. Refresh before retrying.')
    }
    return { ...widget, result: { ...result, hasMore: result.hasMore === true } }
  })
  return { ...data, widgets }
}

function validScheduleCollection(data) {
  return data && typeof data.provider === 'string' && typeof data.deliveryAvailable === 'boolean' && Array.isArray(data.schedules) && data.schedules.length <= 20 && Array.isArray(data.deliveryRuns) && data.deliveryRuns.length <= 20
}

export async function listReportSchedules({ signal } = {}) {
  const payload = await apiRequest('/api/report-schedules', { fallbackMessage: 'Unable to load scheduled report delivery.', signal })
  const data = payload?.data
  if (!validScheduleCollection(data) || data.schedules.some((schedule) => !Number.isInteger(schedule.id) || schedule.id <= 0 || !Number.isInteger(schedule.revision) || schedule.revision <= 0 || !Array.isArray(schedule.recipients) || schedule.recipients.length > 10) || data.deliveryRuns.some((run) => !Number.isInteger(run.id) || run.id <= 0 || !Array.isArray(run.recipients) || run.recipients.length > 10)) {
    throw new Error('The server returned invalid scheduled report delivery state. Refresh before retrying.')
  }
  return data
}

export async function upsertReportSchedule(definitionId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/report-definitions/${definitionId}/schedule`, { method: 'PUT', body: input, fallbackMessage: 'Unable to save scheduled report delivery.', signal })
  const schedule = payload?.data?.schedule
  if (!schedule || schedule.reportDefinitionId !== definitionId || !Number.isInteger(schedule.revision) || schedule.revision < Math.max(1, input.revision) || !Array.isArray(schedule.recipients)) {
    throw new Error('The scheduled report response was invalid. Refresh before retrying.')
  }
  return schedule
}

export async function resolveReportRecipientDelivery(deliveryId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/report-recipient-deliveries/${deliveryId}/resolve`, { method: 'POST', body: input, fallbackMessage: 'Unable to resolve scheduled report delivery.', signal })
  const run = payload?.data?.deliveryRun
  if (!run || !Number.isInteger(run.id) || run.id <= 0 || !Array.isArray(run.recipients) || !run.recipients.some((delivery) => delivery.id === deliveryId)) {
    throw new Error('The delivery recovery response was invalid. Refresh before retrying.')
  }
  return run
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
    closeReasons: Array.isArray(data.closeReasons) ? data.closeReasons : [],
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

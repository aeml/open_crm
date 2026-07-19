import { apiRequest, apiURL } from './api'

export async function listDealPipelines({ signal } = {}) {
  const payload = await apiRequest('/api/deal-pipelines', { fallbackMessage: 'Unable to load deal pipelines.', signal })

  return payload?.data?.pipelines || []
}

export async function createDealPipeline(input, { signal } = {}) {
  const payload = await apiRequest('/api/deal-pipelines', { method: 'POST', body: input, fallbackMessage: 'Unable to create deal pipeline.', signal })

  return payload?.data?.pipeline
}

export async function updateDealPipeline(pipelineId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deal-pipelines/${pipelineId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update deal pipeline.', signal })
  return payload?.data?.pipeline
}

export async function createDealStage(pipelineId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deal-pipelines/${pipelineId}/stages`, { method: 'POST', body: input, fallbackMessage: 'Unable to create deal stage.', signal })
  return payload?.data?.pipeline
}

export async function updateDealStageDefinition(pipelineId, stageId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deal-pipelines/${pipelineId}/stages/${stageId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update deal stage.', signal })
  return payload?.data?.pipeline
}

export async function reorderDealStages(pipelineId, stageIds, { signal } = {}) {
  const payload = await apiRequest(`/api/deal-pipelines/${pipelineId}/stages/order`, { method: 'PUT', body: { stageIds }, fallbackMessage: 'Unable to reorder deal stages.', signal })
  return payload?.data?.pipeline
}

export async function listDealStages({ signal } = {}) {
  const payload = await apiRequest('/api/deal-stages', { fallbackMessage: 'Unable to load deal stages.', signal })

  return payload?.data?.stages || []
}

export async function listDeals(query = {}, { signal } = {}) {
  const params = new URLSearchParams()
  if (query.search) params.set('q', query.search)
  if (query.pipelineId) params.set('pipelineId', String(query.pipelineId))
  if (query.stageId) params.set('stageId', String(query.stageId))
  if (query.unassigned) params.set('unassigned', 'true')
  else if (query.ownerUserId) params.set('ownerUserId', String(query.ownerUserId))
  if (query.companyId) params.set('companyId', String(query.companyId))
  if (query.primaryContactId) params.set('primaryContactId', String(query.primaryContactId))
  if (query.closeFrom) params.set('closeFrom', query.closeFrom)
  if (query.closeTo) params.set('closeTo', query.closeTo)
  const suffix = params.toString() ? `?${params.toString()}` : ''

  const payload = await apiRequest(`/api/deals${suffix}`, { fallbackMessage: 'Unable to load deals.', signal })

  return payload?.data || { deals: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' } }
}

export async function createDeal(input, { signal } = {}) {
  const payload = await apiRequest('/api/deals', { method: 'POST', body: input, fallbackMessage: 'Unable to create deal.', signal })

  return payload?.data
}

export async function getDeal(dealID, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}`, { fallbackMessage: 'Unable to load deal.', signal })

  return payload?.data
}

export async function updateDeal(dealID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update deal.', signal })

  return payload?.data
}

export async function replaceDealLineItems(dealID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/line-items`, { method: 'PUT', body: input, fallbackMessage: 'Unable to update deal line items.', signal })

  return payload?.data
}

export async function createDealSignatureRequest(dealID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/signature-requests`, { method: 'POST', body: input, fallbackMessage: 'Unable to create proposal tracking.', signal })

  return payload?.data
}

export async function updateDealSignatureRequestStatus(dealID, requestID, status, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/signature-requests/${requestID}`, { method: 'PATCH', body: { status }, fallbackMessage: 'Unable to update proposal tracking.', signal })

  return payload?.data
}

export async function archiveDeal(dealID, { signal } = {}) {
  return apiRequest(`/api/deals/${dealID}`, { method: 'DELETE', fallbackMessage: 'Unable to archive deal.', signal })
}

export async function sendDealEmail(dealID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/email`, { method: 'POST', body: input, fallbackMessage: 'Unable to send email.', signal })

  return payload?.data
}

export async function updateDealStage(dealID, stageId, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/stage`, { method: 'PATCH', body: { stageId }, fallbackMessage: 'Unable to move deal.', signal })

  return payload?.data
}

export function dealsExportURL(query = {}) {
  const params = new URLSearchParams()
  if (query.search) params.set('q', query.search)
  if (query.pipelineId) params.set('pipelineId', String(query.pipelineId))
  if (query.stageId) params.set('stageId', String(query.stageId))
  if (query.ownerUserId) params.set('ownerUserId', String(query.ownerUserId))
  if (query.companyId) params.set('companyId', String(query.companyId))
  if (query.primaryContactId) params.set('primaryContactId', String(query.primaryContactId))
  if (query.closeFrom) params.set('closeFrom', query.closeFrom)
  if (query.closeTo) params.set('closeTo', query.closeTo)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return apiURL(`/api/export/deals${suffix}`)
}

export function quotePDFURL(dealID) {
  return apiURL(`/api/deals/${dealID}/quote.pdf`)
}

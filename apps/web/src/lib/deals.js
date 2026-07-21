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

export async function finalizeDealQuote(dealID, input, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/quotes`, {
    method: 'POST',
    body: input,
    headers: { 'Idempotency-Key': idempotencyKey },
    fallbackMessage: 'Unable to finalize quote.',
    signal
  })

  return payload?.data?.quote
}

export async function deliverDealQuote(dealID, quoteID, input, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/quotes/${quoteID}/deliveries`, {
    method: 'POST',
    body: input,
    headers: { 'Idempotency-Key': idempotencyKey },
    fallbackMessage: 'Unable to deliver quote.',
    signal
  })
  return payload?.data?.delivery
}

export async function resolveDealQuoteDelivery(deliveryID, resolution, { signal } = {}) {
  const payload = await apiRequest(`/api/deal-quote-deliveries/${deliveryID}/resolve`, {
    method: 'POST', body: { resolution }, fallbackMessage: 'Unable to resolve quote delivery.', signal
  })
  return payload?.data?.delivery
}

export async function getPublicDealQuote(token, { signal } = {}) {
  const payload = await apiRequest(`/api/public/quotes/${encodeURIComponent(token)}`, { fallbackMessage: 'Unable to load quote.', signal })
  return payload?.data?.quote
}

export async function confirmPublicDealQuoteReceipt(token, { signal } = {}) {
  const payload = await apiRequest(`/api/public/quotes/${encodeURIComponent(token)}/receipt`, {
    method: 'POST', fallbackMessage: 'Unable to confirm quote receipt.', signal
  })
  return payload?.data?.quote
}

export async function updatePublicDealQuote(token, action, input, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest(`/api/public/quotes/${encodeURIComponent(token)}/${action}`, {
    method: 'POST', body: input, headers: { 'Idempotency-Key': idempotencyKey }, fallbackMessage: `Unable to ${action === 'signature' ? 'sign' : 'decline'} quote.`, signal
  })
  return payload?.data?.quote
}

export async function voidDealSignatureRequest(dealID, requestID, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/signature-requests/${requestID}/void`, { method: 'POST', fallbackMessage: 'Unable to void signature request.', signal })
  return payload?.data
}

export async function convertSignedQuoteToWon(dealID, requestID, input, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/signature-requests/${requestID}/convert-to-won`, {
    method: 'POST', body: input, headers: { 'Idempotency-Key': idempotencyKey }, fallbackMessage: 'Unable to convert signed quote to a won deal.', signal
  })
  return payload?.data
}

export async function archiveDeal(dealID, { signal } = {}) {
  return apiRequest(`/api/deals/${dealID}`, { method: 'DELETE', fallbackMessage: 'Unable to archive deal.', signal })
}

export async function sendDealEmail(dealID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/email`, { method: 'POST', body: input, fallbackMessage: 'Unable to send email.', signal })

  return payload?.data
}

export async function updateDealStage(dealID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/deals/${dealID}/stage`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to move deal.', signal })

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

export function quoteVersionPDFURL(dealID, quoteID) {
  return apiURL(`/api/deals/${dealID}/quotes/${quoteID}/pdf`)
}

export function publicQuotePDFURL(token) {
  return apiURL(`/api/public/quotes/${encodeURIComponent(token)}/pdf`)
}

export function publicSignatureCertificateURL(token) {
  return apiURL(`/api/public/quotes/${encodeURIComponent(token)}/signature-certificate`)
}

export function signatureCertificateURL(dealID, requestID) {
  return apiURL(`/api/deals/${dealID}/signature-requests/${requestID}/certificate`)
}

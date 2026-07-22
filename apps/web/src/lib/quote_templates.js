import { apiRequest } from './api'

export async function listQuoteTemplatePage({ search = '', status = 'all', page = 1, pageSize = 50, signal } = {}) {
  const query = new URLSearchParams()
  if (search) query.set('q', search)
  if (status && status !== 'all') query.set('status', status)
  query.set('page', String(page))
  query.set('pageSize', String(pageSize))
  const payload = await apiRequest(`/api/quote-templates?${query.toString()}`, {
    fallbackMessage: 'Unable to load quote templates.', signal
  })
  const data = payload?.data || {}
  const templates = data.templates || []
  return { templates, meta: data.meta || { page, pageSize, total: templates.length } }
}

export async function listQuoteTemplates({ signal } = {}) {
  const templatesById = new Map()
  let expectedTotal = null
  for (let page = 1; page <= 501; page += 1) {
    const result = await listQuoteTemplatePage({ status: 'active', page, pageSize: 100, signal })
    const total = Number(result.meta?.total)
    if (!Number.isSafeInteger(total) || total < 0 || (expectedTotal !== null && total !== expectedTotal)) {
      throw new Error('The quote template catalog changed while quote options were loading. Try again.')
    }
    expectedTotal = total
    result.templates.forEach((template) => templatesById.set(template.id, template))
    if (templatesById.size >= expectedTotal) return [...templatesById.values()]
    if (result.templates.length === 0) break
  }
  throw new Error('The complete active quote template catalog could not be loaded. Archive legacy overflow and try again.')
}

export async function getQuoteTemplatePolicy({ signal } = {}) {
  const payload = await apiRequest('/api/quote-templates/policy', {
    fallbackMessage: 'Unable to load quote approval policy.', signal
  })
  return payload?.data?.policy || { approvalRequired: false, activeApprovers: 0 }
}

export async function listQuoteTemplateMergeTokens({ signal } = {}) {
  const payload = await apiRequest('/api/quote-templates/merge-tokens', {
    fallbackMessage: 'Unable to load quote template merge fields.', signal
  })
  return payload?.data?.tokens || []
}

export async function createQuoteTemplate(input, { signal } = {}) {
  const payload = await apiRequest('/api/quote-templates', {
    method: 'POST', body: input, fallbackMessage: 'Unable to create quote template.', signal
  })
  return payload?.data?.template
}

export async function updateQuoteTemplate(templateID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/quote-templates/${templateID}`, {
    method: 'PATCH', body: input, fallbackMessage: 'Unable to update quote template.', signal
  })
  return payload?.data?.template
}

export async function archiveQuoteTemplate(templateID, revision, { signal } = {}) {
  const payload = await apiRequest(`/api/quote-templates/${templateID}?revision=${encodeURIComponent(revision)}`, {
    method: 'DELETE', fallbackMessage: 'Unable to archive quote template.', signal
  })
  return payload?.data?.template
}

export async function updateQuoteTemplatePolicy(approvalRequired, { signal } = {}) {
  const payload = await apiRequest('/api/quote-templates/policy', {
    method: 'PUT', body: { approvalRequired }, fallbackMessage: 'Unable to update quote approval policy.', signal
  })
  return payload?.data?.policy
}

export async function listPendingQuoteApprovals({ signal } = {}) {
  const payload = await apiRequest('/api/deal-quote-approvals?status=pending', {
    fallbackMessage: 'Unable to load pending quote approvals.', signal
  })
  return payload?.data?.approvals || []
}

export async function loadQuotePreparation({ signal } = {}) {
  const [templates, policy] = await Promise.all([
    listQuoteTemplates({ signal }),
    getQuoteTemplatePolicy({ signal })
  ])
  return { templates, policy }
}

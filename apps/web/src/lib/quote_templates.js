import { apiRequest } from './api'

export async function listQuoteTemplates({ signal } = {}) {
  const payload = await apiRequest('/api/quote-templates', {
    fallbackMessage: 'Unable to load quote templates.', signal
  })
  return payload?.data?.templates || []
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

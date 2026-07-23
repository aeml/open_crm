import { apiRequest } from './api'

export async function listLeadScoringRules({ signal } = {}) {
  const payload = await apiRequest('/api/lead-scoring-rules', { fallbackMessage: 'Unable to load lead scoring rules.', signal })

  return {
    rules: payload?.data?.rules || [],
    capacity: payload?.data?.capacity || { maxRules: 100 },
  }
}

export async function createLeadScoringRule(input, { signal } = {}) {
  const payload = await apiRequest('/api/lead-scoring-rules', { method: 'POST', body: input, fallbackMessage: 'Unable to save lead scoring rule.', signal })

  return payload?.data?.rule
}

export async function updateLeadScoringRule(ruleId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/lead-scoring-rules/${ruleId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update lead scoring rule.', signal })

  return payload?.data?.rule
}

export async function evaluateContactLeadScore(contactId, { signal } = {}) {
  const payload = await apiRequest(`/api/contacts/${contactId}/lead-score`, { method: 'POST', fallbackMessage: 'Unable to evaluate lead score.', signal })

  return payload?.data?.evaluation
}

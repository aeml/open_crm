import { apiRequest } from './api'

export async function listEmailSequences({ signal } = {}) {
  const payload = await apiRequest('/api/email-sequences', { fallbackMessage: 'Unable to load email sequences.', signal })

  return payload?.data?.sequences || []
}

export async function createEmailSequence(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-sequences', { method: 'POST', body: input, fallbackMessage: 'Unable to save email sequence.', signal })

  return payload?.data?.sequence
}

export async function updateEmailSequence(sequenceId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-sequences/${sequenceId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update email sequence.', signal })

  return payload?.data?.sequence
}

export async function deleteEmailSequence(sequenceId, { signal } = {}) {
  await apiRequest(`/api/email-sequences/${sequenceId}`, { method: 'DELETE', fallbackMessage: 'Unable to delete email sequence.', signal })
}

export async function transitionEmailSequence(sequenceId, action, { signal } = {}) {
  const payload = await apiRequest(`/api/email-sequences/${sequenceId}/${action}`, { method: 'POST', fallbackMessage: `Unable to ${action} email sequence.`, signal })

  return payload?.data?.sequence
}

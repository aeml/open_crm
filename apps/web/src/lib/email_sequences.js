import { apiRequest } from './api'
import { loadCompleteCatalog } from './complete_catalog'

export async function listEmailSequencePage({ search = '', status = 'all', page = 1, pageSize = 50, signal } = {}) {
  const query = new URLSearchParams()
  if (search) query.set('q', search)
  if (status && status !== 'all') query.set('status', status)
  query.set('page', String(page))
  query.set('pageSize', String(pageSize))
  const payload = await apiRequest(`/api/email-sequences?${query.toString()}`, { fallbackMessage: 'Unable to load email sequences.', signal })

  const data = payload?.data || {}
  const sequences = data.sequences || []
  return { sequences, meta: data.meta || { page, pageSize, total: sequences.length } }
}

export async function listEmailSequences({ signal } = {}) {
  return loadCompleteCatalog(
    ({ page, pageSize }) => listEmailSequencePage({ status: 'active', page, pageSize, signal }),
    'sequences',
    'The active email sequence catalog changed while options were loading. Try again.',
    'The complete active email sequence catalog could not be loaded. Pause legacy overflow and try again.'
  )
}

export async function createEmailSequence(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-sequences', { method: 'POST', body: input, fallbackMessage: 'Unable to save email sequence.', signal })

  return payload?.data?.sequence
}

export async function updateEmailSequence(sequenceId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-sequences/${sequenceId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update email sequence.', signal })

  return payload?.data?.sequence
}

export async function deleteEmailSequence(sequenceId, revision, { signal } = {}) {
  await apiRequest(`/api/email-sequences/${sequenceId}?revision=${encodeURIComponent(revision)}`, { method: 'DELETE', fallbackMessage: 'Unable to delete email sequence.', signal })
}

export async function transitionEmailSequence(sequenceId, action, revision, { signal } = {}) {
  const query = action === 'approve' ? `?revision=${encodeURIComponent(revision)}` : ''
  const payload = await apiRequest(`/api/email-sequences/${sequenceId}/${action}${query}`, { method: 'POST', fallbackMessage: `Unable to ${action} email sequence.`, signal })

  return payload?.data?.sequence
}

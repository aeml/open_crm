import { apiRequest } from './api'

export async function listEmailSequenceEnrollments({ contactId }, { signal } = {}) {
  const params = new URLSearchParams()
  if (contactId) {
    params.set('contactId', String(contactId))
  }
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/email-sequence-enrollments${suffix}`, { fallbackMessage: 'Unable to load email sequence enrollments.', signal })

  return payload?.data?.enrollments || []
}

export async function listEmailSequenceEnrollmentPage({ sequenceId, cursor = '', limit = 50 }, { signal } = {}) {
  const params = new URLSearchParams({ sequenceId: String(sequenceId), limit: String(limit) })
  if (cursor) {
    params.set('cursor', cursor)
  }
  const payload = await apiRequest(`/api/email-sequence-enrollments?${params.toString()}`, { fallbackMessage: 'Unable to load sequence enrollment history.', signal })

  return {
    enrollments: payload?.data?.enrollments || [],
    meta: payload?.data?.meta || { limit, hasMore: false, nextCursor: '' }
  }
}

export async function createEmailSequenceEnrollment(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-sequence-enrollments', { method: 'POST', body: input, fallbackMessage: 'Unable to enroll contact in sequence.', signal })

  return payload?.data?.enrollment
}

export async function cancelEmailSequenceEnrollment(enrollmentId, { signal } = {}) {
  await apiRequest(`/api/email-sequence-enrollments/${enrollmentId}`, { method: 'DELETE', fallbackMessage: 'Unable to cancel email sequence enrollment.', signal })
}

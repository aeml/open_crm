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

export async function createEmailSequenceEnrollment(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-sequence-enrollments', { method: 'POST', body: input, fallbackMessage: 'Unable to enroll contact in sequence.', signal })

  return payload?.data?.enrollment
}

export async function cancelEmailSequenceEnrollment(enrollmentId, { signal } = {}) {
  await apiRequest(`/api/email-sequence-enrollments/${enrollmentId}`, { method: 'DELETE', fallbackMessage: 'Unable to cancel email sequence enrollment.', signal })
}

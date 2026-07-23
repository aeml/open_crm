import { apiRequest, apiURL } from './api'
import { loadCompleteCatalog } from './complete_catalog'

export async function listLeadCaptureFormPage({ status = 'all', page = 1, pageSize = 50, signal } = {}) {
  const query = new URLSearchParams()
  if (status && status !== 'all') query.set('status', status)
  if (page) query.set('page', String(page))
  if (pageSize) query.set('pageSize', String(pageSize))
  const payload = await apiRequest(`/api/lead-capture-forms?${query.toString()}`, { fallbackMessage: 'Unable to load lead forms.', signal })
  const data = payload?.data || {}

  return { forms: data.forms || [], meta: data.meta || { page, pageSize, total: (data.forms || []).length } }
}

export async function listLeadCaptureForms({ status = 'all', signal } = {}) {
  return loadCompleteCatalog(
    ({ page, pageSize }) => listLeadCaptureFormPage({ status, page, pageSize, signal }),
    'forms',
    'The lead form catalog changed while options were loading. Try again.',
    'The complete lead form catalog could not be loaded. Review retained form history and try again.',
    true
  )
}

export async function createLeadCaptureForm(input, { signal } = {}) {
  const payload = await apiRequest('/api/lead-capture-forms', { method: 'POST', body: input, fallbackMessage: 'Unable to save lead form.', signal })

  return payload?.data?.form
}

export async function updateLeadCaptureForm(formId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/lead-capture-forms/${formId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update lead form.', signal })

  return payload?.data?.form
}

export async function listLeadSubmissionReviews({ status = 'unreviewed', formId = '', cursor = '', limit, signal } = {}) {
	const query = new URLSearchParams()
	if (status) query.set('status', status)
	if (formId) query.set('formId', String(formId))
	if (cursor) query.set('cursor', cursor)
	if (limit) query.set('limit', String(limit))
	const suffix = query.toString() ? `?${query.toString()}` : ''
	const payload = await apiRequest(`/api/lead-capture-submissions${suffix}`, {
		fallbackMessage: 'Unable to load lead submissions.',
		signal
	})

	return payload?.data || { submissions: [], counts: { unreviewed: 0, legitimate: 0, spam: 0 }, limit: 50 }
}

export async function reviewLeadSubmission(submissionId, input, idempotencyKey, { signal } = {}) {
	const payload = await apiRequest(`/api/lead-capture-submissions/${submissionId}/review`, {
		method: 'POST',
		body: input,
		headers: { 'Idempotency-Key': idempotencyKey },
		fallbackMessage: 'Unable to review lead submission.',
		signal
	})

	return payload?.data?.submission
}

export function publicLeadCaptureFormSubmitURL(publicId) {
  return apiURL(`/api/public/lead-capture-forms/${encodeURIComponent(publicId)}/submissions`)
}

export function publicLeadCaptureFormChallengeURL(publicId) {
  return apiURL(`/api/public/lead-capture-forms/${encodeURIComponent(publicId)}/challenge`)
}

export async function issuePublicLeadSubmissionChallenge(publicId, { signal } = {}) {
  const payload = await apiRequest(`/api/public/lead-capture-forms/${encodeURIComponent(publicId)}/challenge`, {
    method: 'POST',
    credentials: 'omit',
    fallbackMessage: 'Unable to prepare the form.',
    signal
  })

  return payload?.data?.challenge
}

export async function waitForPublicLeadChallenge(challenge) {
  const notBefore = Date.parse(challenge?.notBefore || '')
  if (!Number.isFinite(notBefore)) return
  const waitMilliseconds = Math.max(0, Math.min(5000, notBefore - Date.now()))
  if (waitMilliseconds > 0) {
    await new Promise((resolve) => setTimeout(resolve, waitMilliseconds))
  }
}

export async function submitPublicLeadCaptureForm(publicId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/public/lead-capture-forms/${encodeURIComponent(publicId)}/submissions`, { method: 'POST', body: input, credentials: 'omit', fallbackMessage: 'Unable to submit the form.', signal })

  return payload?.data
}

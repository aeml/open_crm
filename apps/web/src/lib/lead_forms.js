import { apiRequest, apiURL } from './api'

export async function listLeadCaptureForms({ signal } = {}) {
  const payload = await apiRequest('/api/lead-capture-forms', { fallbackMessage: 'Unable to load lead forms.', signal })

  return payload?.data?.forms || []
}

export async function createLeadCaptureForm(input, { signal } = {}) {
  const payload = await apiRequest('/api/lead-capture-forms', { method: 'POST', body: input, fallbackMessage: 'Unable to save lead form.', signal })

  return payload?.data?.form
}

export async function updateLeadCaptureForm(formId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/lead-capture-forms/${formId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update lead form.', signal })

  return payload?.data?.form
}

export async function listLeadSubmissionReviews({ status = 'unreviewed', formId = '', signal } = {}) {
	const query = new URLSearchParams()
	if (status) query.set('status', status)
	if (formId) query.set('formId', String(formId))
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
  const payload = await apiRequest(`/api/public/lead-capture-forms/${encodeURIComponent(publicId)}/submissions`, { method: 'POST', body: input, fallbackMessage: 'Unable to submit the form.', signal })

  return payload?.data
}

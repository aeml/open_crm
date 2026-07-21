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

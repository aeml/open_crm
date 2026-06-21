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

export async function submitPublicLeadCaptureForm(publicId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/public/lead-capture-forms/${encodeURIComponent(publicId)}/submissions`, { method: 'POST', body: input, fallbackMessage: 'Unable to submit the form.', signal })

  return payload?.data
}

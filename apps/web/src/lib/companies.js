import { apiRequest, apiURL, getErrorMessage, isAbortError } from './api'

function duplicateCandidate(payload) {
  const candidate = payload?.error?.details?.duplicate
  if (!candidate?.id || !candidate?.entityType) {
    return null
  }
  return {
    id: candidate.id,
    entityType: candidate.entityType,
    label: candidate.label || '',
    reason: candidate.reason || ''
  }
}

function duplicateReasonLabel(message) {
  const match = String(message || '').match(/\(([^()]+)\)\s*$/)
  if (!match?.[1]) {
    return 'possible duplicate'
  }
  return match[1].trim().toLowerCase()
}

function getCompanySaveError(error, fallbackMessage) {
  if (isAbortError(error)) {
    throw error
  }

  const payload = error.payload
  const message = getErrorMessage(payload, fallbackMessage)
  if (error.status === 409) {
    const duplicateError = new Error(`Possible duplicate company: ${duplicateReasonLabel(message)}. Review the existing record before saving again. ${message}`)
    duplicateError.duplicate = duplicateCandidate(payload)
    throw duplicateError
  }
  return new Error(message)
}

export async function listCompanies(query = {}, { signal } = {}) {
  const params = new URLSearchParams()
  const search = typeof query === 'string' ? query : (query.search || '')
  const ownerUserId = typeof query === 'object' ? (query.ownerUserId || 0) : 0
  const unassigned = typeof query === 'object' ? !!query.unassigned : false
  if (search) params.set('q', search)
  if (unassigned) params.set('unassigned', 'true')
  else if (ownerUserId) params.set('ownerUserId', String(ownerUserId))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/companies${suffix}`, { fallbackMessage: 'Unable to load companies.', signal })

  return payload?.data || { companies: [], meta: { page: 1, pageSize: 20, total: 0 } }
}

export async function getCompany(companyID, { signal } = {}) {
  const payload = await apiRequest(`/api/companies/${companyID}`, { fallbackMessage: 'Unable to load company.', signal })

  return payload?.data
}

export async function createCompany(input, { signal } = {}) {
  try {
    const payload = await apiRequest('/api/companies', { method: 'POST', body: input, fallbackMessage: 'Unable to create company.', signal })
    return payload?.data
  } catch (error) {
    throw getCompanySaveError(error, 'Unable to create company.')
  }
}

export async function updateCompany(companyID, input, { signal } = {}) {
  try {
    const payload = await apiRequest(`/api/companies/${companyID}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update company.', signal })
    return payload?.data
  } catch (error) {
    throw getCompanySaveError(error, 'Unable to update company.')
  }
}

export async function archiveCompany(companyID, { signal } = {}) {
  return apiRequest(`/api/companies/${companyID}`, { method: 'DELETE', fallbackMessage: 'Unable to archive company.', signal })
}

export async function sendCompanyEmail(companyID, input, { signal } = {}) {
  const payload = await apiRequest(`/api/companies/${companyID}/email`, { method: 'POST', body: input, fallbackMessage: 'Unable to send email.', signal })

  return payload?.data
}

export function companiesExportURL(search = '') {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return apiURL(`/api/export/companies${suffix}`)
}

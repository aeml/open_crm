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

function getContactSaveError(error, fallbackMessage) {
  if (isAbortError(error)) {
    throw error
  }

  const payload = error.payload
  const message = getErrorMessage(payload, fallbackMessage)
  if (error.status === 409) {
    const duplicateError = new Error(`Possible duplicate contact: ${duplicateReasonLabel(message)}. Review the existing record before saving again. ${message}`)
    duplicateError.duplicate = duplicateCandidate(payload)
    throw duplicateError
  }
  return new Error(message)
}

export async function listContacts(query = {}, { signal } = {}) {
  const params = new URLSearchParams()
  const search = typeof query === 'string' ? query : (query.search || '')
  const ownerUserId = typeof query === 'object' ? (query.ownerUserId || 0) : 0
  const unassigned = typeof query === 'object' ? !!query.unassigned : false
  if (search) params.set('q', search)
  if (unassigned) params.set('unassigned', 'true')
  else if (ownerUserId) params.set('ownerUserId', String(ownerUserId))
  const suffix = params.toString() ? `?${params.toString()}` : ''
  const payload = await apiRequest(`/api/contacts${suffix}`, { fallbackMessage: 'Unable to load contacts.', signal })

  return payload?.data || { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } }
}

export async function getContact(contactID, { signal } = {}) {
  const payload = await apiRequest(`/api/contacts/${contactID}`, { fallbackMessage: 'Unable to load contact.', signal })

  return payload?.data
}

export async function createContact(input, { signal } = {}) {
  try {
    const payload = await apiRequest('/api/contacts', { method: 'POST', body: input, fallbackMessage: 'Unable to create contact.', signal })
    return payload?.data
  } catch (error) {
    throw getContactSaveError(error, 'Unable to create contact.')
  }
}

export async function updateContact(contactID, input, { signal } = {}) {
  try {
    const payload = await apiRequest(`/api/contacts/${contactID}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update contact.', signal })
    return payload?.data
  } catch (error) {
    throw getContactSaveError(error, 'Unable to update contact.')
  }
}

export async function archiveContact(contactID, { signal } = {}) {
  return apiRequest(`/api/contacts/${contactID}`, { method: 'DELETE', fallbackMessage: 'Unable to archive contact.', signal })
}

export function contactsExportURL(search = '') {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return apiURL(`/api/export/contacts${suffix}`)
}

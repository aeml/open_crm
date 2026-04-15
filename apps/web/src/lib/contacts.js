import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

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

function getContactSaveError(response, payload, fallbackMessage) {
  const message = getErrorMessage(payload, fallbackMessage)
  if (response.status === 409) {
    const error = new Error(`Possible duplicate contact: ${duplicateReasonLabel(message)}. Review the existing record before saving again. ${message}`)
    error.duplicate = duplicateCandidate(payload)
    throw error
  }
  return new Error(message)
}

async function readJSON(response) {
  if (response.status === 204) {
    return {}
  }
  return response.json()
}

export async function listContacts(search = '') {
  const suffix = search ? `?q=${encodeURIComponent(search)}` : ''
  const response = await fetch(`${API_BASE_URL}/api/contacts${suffix}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load contacts.'))
  }

  return payload?.data || { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } }
}

export async function getContact(contactID) {
  const response = await fetch(`${API_BASE_URL}/api/contacts/${contactID}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load contact.'))
  }

  return payload?.data
}

export async function createContact(input) {
  const response = await fetch(`${API_BASE_URL}/api/contacts`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw getContactSaveError(response, payload, 'Unable to create contact.')
  }

  return payload?.data
}

export async function updateContact(contactID, input) {
  const response = await fetch(`${API_BASE_URL}/api/contacts/${contactID}`, {
    method: 'PATCH',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw getContactSaveError(response, payload, 'Unable to update contact.')
  }

  return payload?.data
}

export async function archiveContact(contactID) {
  const response = await fetch(`${API_BASE_URL}/api/contacts/${contactID}`, {
    method: 'DELETE',
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to archive contact.'))
  }

  return payload
}

import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

async function readJSON(response) {
  if (!response || typeof response.json !== 'function') {
    return {}
  }
  if (response.status === 204) {
    return {}
  }
  return response.json()
}

export async function listNotes(entityType, entityId) {
  const response = await fetch(`${API_BASE_URL}/api/notes?entityType=${encodeURIComponent(entityType)}&entityId=${encodeURIComponent(entityId)}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load notes.'))
  }

  return payload?.data?.notes || []
}

export async function createNote(input) {
  const response = await fetch(`${API_BASE_URL}/api/notes`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to create note.'))
  }

  return payload?.data
}

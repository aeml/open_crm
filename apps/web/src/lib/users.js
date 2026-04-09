import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

export async function listOrganizationUsers() {
  const response = await fetch(`${API_BASE_URL}/api/users`, {
    credentials: 'include'
  })
  const payload = await response.json()

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load users.'))
  }

  return payload?.data?.users || []
}

export async function createOrganizationUser(input) {
  const response = await fetch(`${API_BASE_URL}/api/users`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await response.json()

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to create user.'))
  }

  return payload?.data?.user
}

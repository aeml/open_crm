import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

async function readJSON(response) {
  if (response.status === 204) {
    return {}
  }
  return response.json()
}

export async function getBusinessProfile() {
  const response = await fetch(`${API_BASE_URL}/api/organization/profile`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load business profile.'))
  }

  return payload?.data?.profile
}

export async function updateBusinessProfile(input) {
  const response = await fetch(`${API_BASE_URL}/api/organization/profile`, {
    method: 'PATCH',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to update business profile.'))
  }

  return payload?.data?.profile
}

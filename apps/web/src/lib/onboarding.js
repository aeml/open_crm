import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

export async function bootstrapWorkspace(input) {
  const response = await fetch(`${API_BASE_URL}/auth/bootstrap`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })

  const payload = await response.json()
  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to create workspace.'))
  }

  return payload.data
}

import { apiRequest } from './api'

export async function updateProfile(input, { signal } = {}) {
  const payload = await apiRequest('/api/me/profile', {
    method: 'PATCH',
    body: input,
    fallbackMessage: 'Unable to update profile.',
    signal
  })
  return payload?.data?.user
}

export async function getPreferences({ signal } = {}) {
  const payload = await apiRequest('/api/me/preferences', {
    fallbackMessage: 'Unable to load preferences.',
    signal
  })
  return payload?.data?.preferences || {}
}

export async function updatePreferences(input, { signal } = {}) {
  const payload = await apiRequest('/api/me/preferences', {
    method: 'PATCH',
    body: input,
    fallbackMessage: 'Unable to save preferences.',
    signal
  })
  return payload?.data?.preferences || {}
}

export async function getSessions({ signal } = {}) {
  const payload = await apiRequest('/api/me/sessions', {
    fallbackMessage: 'Unable to load active sign-ins.',
    signal
  })
  return payload?.data?.sessions || []
}

export async function revokeSession(sessionId, { signal } = {}) {
  await apiRequest(`/api/me/sessions/${sessionId}`, {
    method: 'DELETE',
    fallbackMessage: 'Unable to sign out that session.',
    signal
  })
}

export async function revokeOtherSessions({ signal } = {}) {
  const payload = await apiRequest('/api/me/sessions/others', {
    method: 'DELETE',
    fallbackMessage: 'Unable to sign out other sessions.',
    signal
  })
  return payload?.data?.revoked || 0
}

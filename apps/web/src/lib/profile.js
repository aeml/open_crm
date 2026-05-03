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

import { apiRequest } from './api'

export async function getBusinessProfile() {
  const payload = await apiRequest('/api/organization/profile', { fallbackMessage: 'Unable to load business profile.' })

  return payload?.data?.profile
}

export async function updateBusinessProfile(input) {
  const payload = await apiRequest('/api/organization/profile', { method: 'PATCH', body: input, fallbackMessage: 'Unable to update business profile.' })

  return payload?.data?.profile
}

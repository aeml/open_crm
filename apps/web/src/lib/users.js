import { apiRequest } from './api'

export async function listOrganizationUsers() {
  const payload = await apiRequest('/api/users', { fallbackMessage: 'Unable to load users.' })

  return payload?.data?.users || []
}

export async function createOrganizationUser(input) {
  const payload = await apiRequest('/api/users', { method: 'POST', body: input, fallbackMessage: 'Unable to create user.' })

  return payload?.data?.user
}

export async function completeUserSetup(input) {
  await apiRequest('/auth/setup-password', { method: 'POST', body: input, fallbackMessage: 'Unable to complete password setup.' })
}

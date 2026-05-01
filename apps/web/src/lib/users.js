import { apiRequest } from './api'

export async function listOrganizationUsers({ signal } = {}) {
  const payload = await apiRequest('/api/users', { fallbackMessage: 'Unable to load users.', signal })

  return payload?.data?.users || []
}

export async function createOrganizationUser(input, { signal } = {}) {
  const payload = await apiRequest('/api/users', { method: 'POST', body: input, fallbackMessage: 'Unable to create user.', signal })

  return payload?.data?.user
}

export async function updateOrganizationUserRole(userId, role, { signal } = {}) {
  const payload = await apiRequest(`/api/users/${userId}/role`, { method: 'PATCH', body: { role }, fallbackMessage: 'Unable to update user role.', signal })

  return payload?.data?.user
}

export async function completeUserSetup(input, { signal } = {}) {
  await apiRequest('/auth/setup-password', { method: 'POST', body: input, fallbackMessage: 'Unable to complete password setup.', signal })
}

import { apiRequest } from './api'

export async function listOrganizationUsers({ signal, includeDisabled = false } = {}) {
  const payload = await apiRequest('/api/users', { fallbackMessage: 'Unable to load users.', signal })

  const users = payload?.data?.users || []

  return includeDisabled ? users : users.filter((user) => (user.status || 'active') === 'active')
}

export async function createOrganizationUser(input, { signal } = {}) {
  const payload = await apiRequest('/api/users', { method: 'POST', body: input, fallbackMessage: 'Unable to create user.', signal })

  return payload?.data?.user
}

export async function resendOrganizationUserInvitation(userId, { signal } = {}) {
  const payload = await apiRequest(`/api/users/${userId}/invitation/resend`, { method: 'POST', fallbackMessage: 'Unable to resend invitation.', signal })

  return payload?.data?.user
}

export async function revokeOrganizationUserInvitation(userId, { signal } = {}) {
  const payload = await apiRequest(`/api/users/${userId}/invitation`, { method: 'DELETE', fallbackMessage: 'Unable to revoke invitation.', signal })

  return payload?.data
}

export async function updateOrganizationUserRole(userId, role, { signal } = {}) {
  const payload = await apiRequest(`/api/users/${userId}/role`, { method: 'PATCH', body: { role }, fallbackMessage: 'Unable to update user role.', signal })

  return payload?.data?.user
}

export async function updateOrganizationUserStatus(userId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/users/${userId}/status`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update user access.', signal })

  return payload?.data
}

export async function completeUserSetup(input, { signal } = {}) {
  await apiRequest('/auth/setup-password', { method: 'POST', body: input, fallbackMessage: 'Unable to complete password setup.', signal })
}

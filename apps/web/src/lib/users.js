import { apiRequest } from './api'
import { loadCompleteCatalog } from './complete_catalog'

export async function listOrganizationUsersPage({ search = '', status = 'all', page = 1, pageSize = 50, signal } = {}) {
  const query = new URLSearchParams()
  if (search) query.set('q', search)
  if (status && status !== 'all') query.set('status', status)
  if (page > 1) query.set('page', String(page))
  if (pageSize !== 50) query.set('pageSize', String(pageSize))
  const suffix = query.toString()
  const payload = await apiRequest(`/api/users${suffix ? `?${suffix}` : ''}`, { fallbackMessage: 'Unable to load users.', signal })
  const data = payload?.data || {}
  return { users: data.users || [], meta: data.meta || { page, pageSize, total: data.users?.length || 0 } }
}

export async function listOrganizationUsers({ signal, includeDisabled = false } = {}) {
  const status = includeDisabled ? 'all' : 'active'
  return loadCompleteCatalog(
    ({ page, pageSize }) => listOrganizationUsersPage({ status, page, pageSize, signal }),
    'users',
    'Team access changed while options loaded. Try again.',
    'Unable to load every teammate. Narrow the team catalog and try again.',
    true
  )
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

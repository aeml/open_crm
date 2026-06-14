import { apiRequest } from './api'

export async function getMyEmailAccount({ signal } = {}) {
  const payload = await apiRequest('/api/me/email-account', { fallbackMessage: 'Unable to load email account.', signal })

  return payload?.data || { account: null, configured: false }
}

export async function getMyEmailSyncStatus({ signal } = {}) {
  const payload = await apiRequest('/api/me/email-sync/status', { fallbackMessage: 'Unable to load email sync status.', signal })

  return payload?.data || { account: null, configured: false, connected: false, oauthProviders: [] }
}

export async function saveMyEmailAccount(input, { signal } = {}) {
  const payload = await apiRequest('/api/me/email-account', { method: 'PUT', body: input, fallbackMessage: 'Unable to save email account.', signal })

  return payload?.data?.account
}

export async function deleteMyEmailAccount({ signal } = {}) {
  await apiRequest('/api/me/email-account', { method: 'DELETE', fallbackMessage: 'Unable to remove email account.', signal })
}

export async function getUserEmailAccount(userId, { signal } = {}) {
  const payload = await apiRequest(`/api/users/${userId}/email-account`, { fallbackMessage: 'Unable to load email account.', signal })

  return payload?.data || { account: null, configured: false }
}

export async function saveUserEmailAccount(userId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/users/${userId}/email-account`, { method: 'PUT', body: input, fallbackMessage: 'Unable to save email account.', signal })

  return payload?.data?.account
}

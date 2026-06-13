import { apiRequest } from './api'

export async function getMyEmailAccount({ signal } = {}) {
  const payload = await apiRequest('/api/me/email-account', { fallbackMessage: 'Unable to load email account.', signal })

  return payload?.data || { account: null, configured: false }
}

export async function saveMyEmailAccount(input, { signal } = {}) {
  const payload = await apiRequest('/api/me/email-account', { method: 'PUT', body: input, fallbackMessage: 'Unable to save email account.', signal })

  return payload?.data?.account
}

export async function deleteMyEmailAccount({ signal } = {}) {
  await apiRequest('/api/me/email-account', { method: 'DELETE', fallbackMessage: 'Unable to remove email account.', signal })
}

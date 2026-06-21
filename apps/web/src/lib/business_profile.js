import { apiRequest } from './api'

export async function getBusinessProfile({ signal } = {}) {
  const payload = await apiRequest('/api/organization/profile', { fallbackMessage: 'Unable to load business profile.', signal })

  return payload?.data?.profile
}

export async function updateBusinessProfile(input, { signal } = {}) {
  const payload = await apiRequest('/api/organization/profile', { method: 'PATCH', body: input, fallbackMessage: 'Unable to update business profile.', signal })

  return payload?.data?.profile
}

export async function upsertExchangeRate(quoteCurrency, input, { signal } = {}) {
  const payload = await apiRequest(`/api/organization/exchange-rates/${quoteCurrency}`, { method: 'PUT', body: input, fallbackMessage: 'Unable to save exchange rate.', signal })

  return payload?.data?.profile
}

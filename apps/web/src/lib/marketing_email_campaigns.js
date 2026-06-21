import { apiRequest } from './api'

export async function listMarketingEmailCampaigns({ signal } = {}) {
  const payload = await apiRequest('/api/marketing-email-campaigns', { fallbackMessage: 'Unable to load marketing campaigns.', signal })

  return payload?.data?.campaigns || []
}

export async function createMarketingEmailCampaign(input, { signal } = {}) {
  const payload = await apiRequest('/api/marketing-email-campaigns', { method: 'POST', body: input, fallbackMessage: 'Unable to save marketing campaign.', signal })

  return payload?.data?.campaign
}

export async function updateMarketingEmailCampaign(campaignId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/marketing-email-campaigns/${campaignId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update marketing campaign.', signal })

  return payload?.data?.campaign
}

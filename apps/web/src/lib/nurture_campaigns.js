import { apiRequest } from './api'

export async function listNurtureCampaigns({ signal } = {}) {
  const payload = await apiRequest('/api/lead-nurture-campaigns', { fallbackMessage: 'Unable to load nurture campaigns.', signal })

  return {
    campaigns: payload?.data?.campaigns || [],
    capacity: payload?.data?.capacity || { maxCampaigns: 100 },
  }
}

export async function createNurtureCampaign(input, { signal } = {}) {
  const payload = await apiRequest('/api/lead-nurture-campaigns', { method: 'POST', body: input, fallbackMessage: 'Unable to save nurture campaign.', signal })

  return payload?.data?.campaign
}

export async function updateNurtureCampaign(campaignId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/lead-nurture-campaigns/${campaignId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update nurture campaign.', signal })

  return payload?.data?.campaign
}

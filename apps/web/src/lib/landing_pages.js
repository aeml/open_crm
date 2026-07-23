import { apiRequest, apiURL } from './api'

export async function listLeadLandingPagePage({ status = 'all', page = 1, pageSize = 50, signal } = {}) {
  const query = new URLSearchParams()
  if (status && status !== 'all') query.set('status', status)
  query.set('page', String(page))
  query.set('pageSize', String(pageSize))
  const payload = await apiRequest(`/api/lead-landing-pages?${query.toString()}`, { fallbackMessage: 'Unable to load landing pages.', signal })
  const data = payload?.data || {}

  return { pages: data.pages || [], meta: data.meta || { page, pageSize, total: (data.pages || []).length } }
}

export async function createLeadLandingPage(input, { signal } = {}) {
  const payload = await apiRequest('/api/lead-landing-pages', { method: 'POST', body: input, fallbackMessage: 'Unable to save landing page.', signal })

  return payload?.data?.page
}

export async function updateLeadLandingPage(pageId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/lead-landing-pages/${pageId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update landing page.', signal })

  return payload?.data?.page
}

export async function getPublicLeadLandingPage(slug, { signal } = {}) {
  const payload = await apiRequest(`/api/public/landing-pages/${encodeURIComponent(slug)}`, { fallbackMessage: 'Unable to load landing page.', signal })

  return payload?.data
}

export function publicLeadLandingPageURL(slug) {
  if (typeof window === 'undefined') {
    return `/lp/${slug || ''}`
  }
  return `${window.location.origin}/lp/${slug || ''}`
}

export function publicLeadLandingPageAPIURL(slug) {
  return apiURL(`/api/public/landing-pages/${encodeURIComponent(slug)}`)
}

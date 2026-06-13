import { apiRequest } from './api'

export async function getEntitlements({ signal } = {}) {
  const payload = await apiRequest('/api/billing/entitlements', { fallbackMessage: 'Unable to load plan details.', signal })

  return payload?.data?.entitlements || null
}

export async function listPlans({ signal } = {}) {
  const payload = await apiRequest('/api/billing/plans', { fallbackMessage: 'Unable to load plans.', signal })

  return payload?.data?.plans || []
}

export async function changePlan(plan, { signal } = {}) {
  const payload = await apiRequest('/api/billing/change-plan', { method: 'POST', body: { plan }, fallbackMessage: 'Unable to change plan.', signal })

  return payload?.data?.entitlements || null
}

const featureLabels = {
  saved_views: 'Saved views',
  csv_import: 'CSV import',
  csv_export: 'CSV export',
  email_sync: 'Email sync',
  automation: 'Workflow automation',
  custom_fields: 'Custom fields',
  api_access: 'API access',
  advanced_reporting: 'Advanced reporting',
  sso: 'Single sign-on (SSO)'
}

export function featureLabel(key) {
  return featureLabels[key] || key
}

export function formatLimit(usage) {
  if (!usage) {
    return ''
  }
  if (usage.unlimited) {
    return `${usage.used.toLocaleString()} / Unlimited`
  }
  return `${usage.used.toLocaleString()} / ${usage.limit.toLocaleString()}`
}

export function formatPrice(plan) {
  if (!plan) {
    return ''
  }
  if (plan.key === 'enterprise') {
    return 'Contact sales'
  }
  if (!plan.monthlyPriceUsd) {
    return 'Free'
  }
  return `$${plan.monthlyPriceUsd}/mo`
}

export function trialBanner(subscription) {
  if (!subscription) {
    return ''
  }
  if (subscription.inTrial) {
    const days = subscription.trialDaysLeft
    return `${days} day${days === 1 ? '' : 's'} left in your trial`
  }
  if (subscription.status === 'trialing') {
    return 'Your trial has ended. Choose a plan to continue.'
  }
  if (subscription.status === 'past_due') {
    return 'Your subscription is past due. Update billing to avoid interruption.'
  }
  if (subscription.status === 'canceled') {
    return 'Your subscription is canceled.'
  }
  return ''
}

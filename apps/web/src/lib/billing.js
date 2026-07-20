import { apiRequest, apiURL } from './api'

export async function getEntitlements({ signal } = {}) {
  const payload = await apiRequest('/api/billing/entitlements', { fallbackMessage: 'Unable to load plan details.', signal })

  return payload?.data?.entitlements || null
}

export async function listPlans({ signal } = {}) {
  const payload = await apiRequest('/api/billing/plans', { fallbackMessage: 'Unable to load plans.', signal })

  return payload?.data?.plans || []
}

export async function getBillingUsage({ signal } = {}) {
  const payload = await apiRequest('/api/billing/usage', { fallbackMessage: 'Unable to reconcile measured usage.', signal })

  return payload?.data?.usage || null
}

export async function listBillingInvoices({ signal } = {}) {
  const payload = await apiRequest('/api/billing/invoices', { fallbackMessage: 'Unable to load invoice history.', signal })

  return payload?.data?.invoices || []
}

export async function changePlan(plan, { signal } = {}) {
  const payload = await apiRequest('/api/billing/change-plan', { method: 'POST', body: { plan }, fallbackMessage: 'Unable to change plan.', signal })

  return payload?.data?.entitlements || null
}

export async function createCheckoutSession(plan, idempotencyKey, { signal } = {}) {
  const payload = await apiRequest('/api/billing/checkout-session', {
    method: 'POST',
    body: { plan, idempotencyKey },
    fallbackMessage: 'Unable to open secure checkout.',
    signal
  })
  return payload?.data || null
}

export async function createBillingPortalSession({ signal } = {}) {
  const payload = await apiRequest('/api/billing/portal-session', {
    method: 'POST',
    fallbackMessage: 'Unable to open the billing portal.',
    signal
  })
  return payload?.data || null
}

export async function listWorkspaceExports({ signal } = {}) {
  const payload = await apiRequest('/api/workspace-exports', { fallbackMessage: 'Unable to load workspace exports.', signal })
  return payload?.data?.exports || []
}

export async function requestWorkspaceExport(idempotencyKey) {
  const payload = await apiRequest('/api/workspace-exports', {
    method: 'POST', headers: { 'Idempotency-Key': idempotencyKey }, fallbackMessage: 'Unable to request a workspace export.'
  })
  return payload?.data?.export || null
}

export function workspaceExportDownloadURL(exportID) {
  return apiURL(`/api/workspace-exports/${exportID}/download`)
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

export function formatUsageValue(metric) {
  if (!metric) {
    return ''
  }
  const used = Number(metric.used) || 0
  if (metric.unit !== 'bytes') {
    return `${used.toLocaleString()} ${metric.unit}`
  }
  const units = ['bytes', 'KiB', 'MiB', 'GiB', 'TiB']
  let value = used
  let index = 0
  while (value >= 1024 && index < units.length - 1) {
    value /= 1024
    index += 1
  }
  return `${index === 0 ? value.toLocaleString() : value.toFixed(value >= 10 ? 1 : 2)} ${units[index]}`
}

// Stripe retains two-decimal API representations for ISK and UGX even though
// locale formatters correctly treat the currencies themselves as zero-decimal.
const stripeTwoDecimalCompatibilityCurrencies = new Set(['ISK', 'UGX'])

export function formatInvoiceAmount(amount, currency, provider = '') {
  const value = Number(amount) || 0
  const code = String(currency || '').trim().toUpperCase()
  try {
    const formatter = new Intl.NumberFormat(undefined, { style: 'currency', currency: code })
    const digits = String(provider).toLowerCase() === 'stripe' && stripeTwoDecimalCompatibilityCurrencies.has(code)
      ? 2
      : formatter.resolvedOptions().maximumFractionDigits
    return formatter.format(value / (10 ** digits))
  } catch {
    return `${value.toLocaleString()} ${code || 'minor units'}`
  }
}

export function trialBanner(subscription) {
  if (!subscription || subscription.managed === false) {
    return ''
  }
  if (subscription.suspended) {
    return 'Your hosted subscription is suspended. Use the billing portal to resolve payment or subscription status.'
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
  if (subscription.cancelAtPeriodEnd) {
    const ending = subscription.currentPeriodEnd ? new Date(subscription.currentPeriodEnd).toLocaleDateString() : 'the current period end'
    return `Your subscription is scheduled to cancel on ${ending}.`
  }
  return ''
}

import AxeBuilder from '@axe-core/playwright'
import { expect, test } from '@playwright/test'
import { createHmac } from 'node:crypto'

const apiURL = process.env.OPEN_CRM_E2E_API_URL || 'http://127.0.0.1:8081'
const webURL = process.env.OPEN_CRM_E2E_WEB_URL || 'http://127.0.0.1:4173'
const stripeSandboxURL = process.env.OPEN_CRM_E2E_STRIPE_SANDBOX_URL || 'http://127.0.0.1:2527'
const stripeWebhookSecret = process.env.OPEN_CRM_E2E_STRIPE_WEBHOOK_SECRET || 'whsec_open_crm_e2e'
const hostedBilling = process.env.OPEN_CRM_E2E_BILLING_PROVIDER === 'stripe'
const wcagTags = ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22a', 'wcag22aa']

test.describe.configure({ retries: 0 })
test.skip(!hostedBilling, 'The hosted billing journey requires the isolated Stripe-mode browser harness.')

function uniqueRunID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

async function expectNoAccessibilityViolations(page, surface) {
  const results = await new AxeBuilder({ page }).withTags(wcagTags).analyze()
  const violations = results.violations.map((violation) => ({
    id: violation.id,
    impact: violation.impact,
    help: violation.help,
    helpUrl: violation.helpUrl,
    nodes: violation.nodes.map((node) => ({
      target: node.target,
      html: node.html,
      failureSummary: node.failureSummary
    }))
  }))
  await test.info().attach(`axe-${surface}`, {
    body: JSON.stringify({ url: page.url(), violations }, null, 2),
    contentType: 'application/json'
  })
  expect(violations, `${surface} must have no automated WCAG A/AA violations`).toEqual([])
}

async function deliverStripeWebhook(request, event) {
  const payload = JSON.stringify(event)
  const timestamp = Math.floor(Date.now() / 1000)
  const digest = createHmac('sha256', stripeWebhookSecret).update(`${timestamp}.${payload}`).digest('hex')
  const response = await request.post(`${apiURL}/api/billing/webhooks/stripe`, {
    data: payload,
    headers: {
      'Content-Type': 'application/json',
      'Stripe-Signature': `t=${timestamp},v1=${digest}`
    }
  })
  const body = await response.json()
  expect(response.status(), JSON.stringify(body)).toBe(200)
  expect(body.data).toMatchObject({ accepted: true, eventId: event.id, applied: true })
  return body.data
}

async function stripeSandboxCalls(request) {
  const response = await request.get(`${stripeSandboxURL}/calls`)
  expect(response.status()).toBe(200)
  return (await response.json()).calls
}

async function workspaceAccess(request) {
  const response = await request.get(`${apiURL}/auth/me`)
  expect(response.status()).toBe(200)
  return (await response.json()).data.workspaceAccess.state
}

async function createContact(request, runID, prefix) {
  return request.post(`${apiURL}/api/contacts`, {
    data: {
      firstName: prefix,
      lastName: 'Hosted Billing',
      email: `${prefix.toLowerCase().replaceAll(' ', '-')}-${runID}@example.test`,
      status: 'lead'
    }
  })
}

test('hosted customer can activate, enter dunning, recover, cancel, and export through Stripe-shaped boundaries', async ({ page }) => {
  test.setTimeout(90_000)
  const runID = uniqueRunID()
  const ownerEmail = `hosted-owner-${runID}@example.test`
  const resetStripe = await page.request.delete(`${stripeSandboxURL}/calls`)
  expect(resetStripe.status()).toBe(200)

  await page.goto('/bootstrap')
  await page.getByLabel('Company name').fill(`Hosted Billing Workspace ${runID}`)
  await page.getByLabel('Business type').selectOption('general')
  await page.getByLabel('First name').fill('Hosted')
  await page.getByLabel('Last name').fill('Owner')
  await page.getByLabel('Email').fill(ownerEmail)
  await page.getByLabel('Password').fill('Hosted-Billing-Secure-37!')
  await page.getByRole('button', { name: 'Create workspace' }).click()
  await expect(page.getByRole('heading', { name: 'Check your email' })).toBeVisible()
  await expect(page.getByText('your 14-day trial starts only after verification', { exact: false })).toBeVisible()
  await page.getByRole('link', { name: 'Verify email locally' }).click()
  await expect(page).toHaveURL(/\/dashboard$/)

  const sessionResponse = await page.request.get(`${apiURL}/auth/me`)
  expect(sessionResponse.status()).toBe(200)
  const session = (await sessionResponse.json()).data
  const organizationID = session.organization.id
  const customerID = `cus_e2e_${organizationID}`
  const subscriptionID = `sub_e2e_${organizationID}`
  const eventTime = Math.floor(Date.now() / 1000) - 30
  const periodEnd = eventTime + 30 * 24 * 60 * 60

  await page.goto('/settings/billing')
  await expect(page.getByText('Free · Managed by Stripe', { exact: true })).toBeVisible()
  await expect(page.getByText(/days left in your trial/i)).toBeVisible()
  await expect(workspaceAccess(page.request)).resolves.toBe('writable')
  await expectNoAccessibilityViolations(page, 'hosted-trial-billing')

  const proPlan = page.getByRole('list', { name: 'Available plans' }).getByRole('listitem').filter({ has: page.getByRole('heading', { name: 'Pro' }) })
  await Promise.all([
    page.waitForURL(new RegExp(`^${stripeSandboxURL.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}/checkout/`)),
    proPlan.getByRole('button', { name: 'Continue to secure checkout' }).click()
  ])
  await expect(page.getByRole('heading', { name: 'Stripe test checkout' })).toBeVisible()
  await expect(page.getByText('Only a signed provider webhook can do that.', { exact: false })).toBeVisible()

  let providerCalls = await stripeSandboxCalls(page.request)
  expect(providerCalls).toHaveLength(1)
  expect(providerCalls[0]).toMatchObject({
    type: 'checkout',
    organizationID: String(organizationID),
    plan: 'pro',
    customerEmail: ownerEmail,
    price: 'price_open_crm_e2e_pro',
    successURL: `${webURL}/settings/billing?checkout=success`,
    cancelURL: `${webURL}/settings/billing?checkout=canceled`,
    idempotencyPresent: true
  })

  await page.goto('/settings/billing?checkout=success')
  await expect(page.getByText('Checkout returned successfully.', { exact: false })).toBeVisible()
  await expect(page.getByText('Free · Managed by Stripe', { exact: true })).toBeVisible()

  const checkoutEvent = {
    id: `evt_checkout_${runID}`,
    type: 'checkout.session.completed',
    created: eventTime,
    livemode: false,
    data: {
      object: {
        id: 'cs_e2e_1',
        client_reference_id: String(organizationID),
        customer: customerID,
        subscription: subscriptionID,
        payment_status: 'paid',
        metadata: { organization_id: String(organizationID), plan_key: 'pro' }
      }
    }
  }
  expect((await deliverStripeWebhook(page.request, checkoutEvent)).duplicate).toBe(false)
  expect((await deliverStripeWebhook(page.request, checkoutEvent)).duplicate).toBe(true)
  await page.reload()
  await expect(page.getByText('Free · Managed by Stripe', { exact: true })).toBeVisible()

  await deliverStripeWebhook(page.request, {
    id: `evt_active_${runID}`,
    type: 'customer.subscription.updated',
    created: eventTime + 1,
    livemode: false,
    data: {
      object: {
        id: subscriptionID,
        customer: customerID,
        status: 'active',
        current_period_start: eventTime,
        current_period_end: periodEnd,
        cancel_at_period_end: false,
        metadata: { organization_id: String(organizationID), plan_key: 'pro' }
      }
    }
  })
  await page.reload()
  await expect(page.getByText('Pro · Managed by Stripe', { exact: true })).toBeVisible()
  await expect(page.getByText(/Stripe subscription period/i)).toBeVisible()
  await expect(workspaceAccess(page.request)).resolves.toBe('writable')
  expect((await createContact(page.request, runID, 'Active')).status()).toBe(201)

  await deliverStripeWebhook(page.request, {
    id: `evt_payment_failed_${runID}`,
    type: 'invoice.payment_failed',
    created: eventTime + 2,
    livemode: false,
    data: {
      object: {
        id: `in_failed_${runID}`,
        customer: customerID,
        subscription: subscriptionID,
        status: 'open',
        currency: 'usd',
        amount_due: 4900,
        amount_paid: 0,
        attempted: true,
        attempt_count: 1,
        next_payment_attempt: eventTime + 24 * 60 * 60,
        hosted_invoice_url: 'https://invoice.stripe.test/open',
        invoice_pdf: 'https://invoice.stripe.test/open.pdf',
        created: eventTime
      }
    }
  })
  await page.reload()
  await expect(page.getByText('Your subscription is past due.', { exact: false })).toBeVisible()
  const failedInvoice = page.getByRole('list', { name: 'Invoice and payment history' }).getByRole('listitem').filter({ hasText: `in_failed_${runID}` })
  await expect(failedInvoice).toContainText('open')
  await expect(failedInvoice).toContainText('Next provider retry:')
  expect((await createContact(page.request, runID, 'Past Due Grace')).status()).toBe(201)

  await deliverStripeWebhook(page.request, {
    id: `evt_unpaid_${runID}`,
    type: 'customer.subscription.updated',
    created: eventTime + 3,
    livemode: false,
    data: {
      object: {
        id: subscriptionID,
        customer: customerID,
        status: 'unpaid',
        current_period_end: periodEnd,
        cancel_at_period_end: false,
        metadata: { organization_id: String(organizationID), plan_key: 'pro' }
      }
    }
  })
  await page.reload()
  await expect(page.getByRole('alert').filter({ hasText: 'Workspace is read-only' })).toBeVisible()
  await expect(page.getByText('Your hosted subscription is suspended.', { exact: false })).toBeVisible()
  await expect(workspaceAccess(page.request)).resolves.toBe('read_only')
  const blockedWrite = await createContact(page.request, runID, 'Blocked')
  expect(blockedWrite.status()).toBe(402)
  expect((await blockedWrite.json()).error.code).toBe('SUBSCRIPTION_INACTIVE')
  await expect(failedInvoice).toContainText('open')
  await expectNoAccessibilityViolations(page, 'hosted-suspended-billing')

  await Promise.all([
    page.waitForURL(new RegExp(`^${stripeSandboxURL.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}/portal/`)),
    page.getByRole('button', { name: 'Manage payment method, invoices, or cancellation' }).click()
  ])
  await expect(page.getByRole('heading', { name: 'Stripe test customer portal' })).toBeVisible()
  providerCalls = await stripeSandboxCalls(page.request)
  expect(providerCalls.filter((call) => call.type === 'portal')).toEqual([
    expect.objectContaining({ customerID, returnURL: `${webURL}/settings/billing`, idempotencyPresent: true })
  ])

  await deliverStripeWebhook(page.request, {
    id: `evt_recovered_${runID}`,
    type: 'customer.subscription.updated',
    created: eventTime + 4,
    livemode: false,
    data: {
      object: {
        id: subscriptionID,
        customer: customerID,
        status: 'active',
        current_period_start: eventTime,
        current_period_end: periodEnd,
        cancel_at_period_end: false,
        metadata: { organization_id: String(organizationID), plan_key: 'pro' }
      }
    }
  })
  await page.goto('/settings/billing')
  await expect(page.getByRole('alert').filter({ hasText: 'Workspace is read-only' })).toHaveCount(0)
  await expect(workspaceAccess(page.request)).resolves.toBe('writable')
  expect((await createContact(page.request, runID, 'Recovered')).status()).toBe(201)

  await deliverStripeWebhook(page.request, {
    id: `evt_cancel_scheduled_${runID}`,
    type: 'customer.subscription.updated',
    created: eventTime + 5,
    livemode: false,
    data: {
      object: {
        id: subscriptionID,
        customer: customerID,
        status: 'active',
        current_period_start: eventTime,
        current_period_end: periodEnd,
        cancel_at_period_end: true,
        metadata: { organization_id: String(organizationID), plan_key: 'pro' }
      }
    }
  })
  await page.reload()
  await expect(page.getByText('Your subscription is scheduled to cancel on', { exact: false })).toBeVisible()
  await expect(workspaceAccess(page.request)).resolves.toBe('writable')

  await deliverStripeWebhook(page.request, {
    id: `evt_canceled_${runID}`,
    type: 'customer.subscription.deleted',
    created: eventTime + 6,
    livemode: false,
    data: {
      object: {
        id: subscriptionID,
        customer: customerID,
        status: 'canceled',
        current_period_end: periodEnd,
        cancel_at_period_end: false,
        metadata: { organization_id: String(organizationID), plan_key: 'pro' }
      }
    }
  })
  await page.reload()
  await expect(page.getByRole('alert').filter({ hasText: 'Workspace is read-only' })).toBeVisible()
  await expect(page.getByText('Your subscription is canceled.', { exact: true })).toBeVisible()
  await expect(workspaceAccess(page.request)).resolves.toBe('read_only')
  const canceledWrite = await createContact(page.request, runID, 'Canceled')
  expect(canceledWrite.status()).toBe(402)
  expect((await canceledWrite.json()).error.code).toBe('SUBSCRIPTION_INACTIVE')

  await page.getByRole('button', { name: 'Create workspace export' }).click()
  const portableExport = page.getByRole('list', { name: 'Workspace export history' }).getByRole('listitem').first()
  await expect(portableExport).toContainText('· ready', { timeout: 30_000 })
  const portableDownloadURL = await portableExport.getByRole('link', { name: 'Download ZIP' }).getAttribute('href')
  const portableResponse = await page.request.get(portableDownloadURL)
  expect(portableResponse.status()).toBe(200)
  expect(portableResponse.headers()['content-type']).toContain('application/zip')
  expect(portableResponse.headers()['x-content-sha256']).toMatch(/^[a-f0-9]{64}$/)
  await expectNoAccessibilityViolations(page, 'hosted-canceled-export')
})

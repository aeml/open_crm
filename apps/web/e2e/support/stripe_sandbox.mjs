import http from 'node:http'

const port = Number(process.env.OPEN_CRM_E2E_STRIPE_HTTP_PORT || 2527)
const host = '127.0.0.1'
const origin = `http://${host}:${port}`
const expectedSecret = process.env.OPEN_CRM_E2E_STRIPE_SECRET_KEY || 'sk_test_open_crm_e2e'
const maxBodyBytes = 64 * 1024
const maxCalls = 100

let calls = []
let checkoutSequence = 0
let portalSequence = 0
const subscriptions = new Map()

function respondJSON(response, status, payload) {
  response.statusCode = status
  response.setHeader('Cache-Control', 'no-store')
  response.setHeader('Content-Type', 'application/json; charset=utf-8')
  response.end(JSON.stringify(payload))
}

function respondHTML(response, title, description) {
  response.statusCode = 200
  response.setHeader('Cache-Control', 'no-store')
  response.setHeader('Content-Security-Policy', "default-src 'none'; style-src 'unsafe-inline'")
  response.setHeader('Content-Type', 'text/html; charset=utf-8')
  response.end(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>${title}</title><style>body{font:16px system-ui;max-width:42rem;margin:4rem auto;padding:0 1rem;color:#18202b}main{border:1px solid #ccd5df;border-radius:.75rem;padding:2rem}code{background:#eef2f6;padding:.15rem .3rem}</style></head><body><main><h1>${title}</h1><p>${description}</p><p><strong>Production-equivalent test boundary:</strong> this page never changes Open CRM state. Only a signed provider webhook can do that.</p></main></body></html>`)
}

async function readBody(request) {
  const chunks = []
  let size = 0
  for await (const chunk of request) {
    size += chunk.length
    if (size > maxBodyBytes) throw new Error('request body exceeds sandbox limit')
    chunks.push(chunk)
  }
  return Buffer.concat(chunks).toString('utf8')
}

function recordCall(call) {
  calls.push({ ...call, observedAt: new Date().toISOString() })
  if (calls.length > maxCalls) calls = calls.slice(-maxCalls)
}

function providerRequestIsValid(request) {
  return request.headers.authorization === `Bearer ${expectedSecret}` && request.headers['stripe-version'] === '2024-06-20'
}

const server = http.createServer(async (request, response) => {
  const requestURL = new URL(request.url, origin)

  if (request.method === 'GET' && requestURL.pathname === '/health') {
    respondJSON(response, 200, { status: 'ok' })
    return
  }
  if (request.method === 'GET' && requestURL.pathname === '/calls') {
    respondJSON(response, 200, { calls })
    return
  }
  if (request.method === 'DELETE' && requestURL.pathname === '/calls') {
    calls = []
    checkoutSequence = 0
    portalSequence = 0
    subscriptions.clear()
    respondJSON(response, 200, { calls: [] })
    return
  }
  if (request.method === 'GET' && requestURL.pathname.startsWith('/checkout/')) {
    respondHTML(response, 'Stripe test checkout', 'The Open CRM browser reached the exact server-created Checkout destination.')
    return
  }
  if (request.method === 'GET' && requestURL.pathname.startsWith('/portal/')) {
    respondHTML(response, 'Stripe test customer portal', 'The suspended Open CRM workspace retained its billing-recovery route.')
    return
  }

  if (!providerRequestIsValid(request)) {
    respondJSON(response, 401, { error: { type: 'invalid_request_error', code: 'invalid_api_key', message: 'Stripe sandbox credentials or API version are invalid.' } })
    return
  }

  try {
    if (request.method === 'POST' && requestURL.pathname === '/v1/checkout/sessions') {
      const form = new URLSearchParams(await readBody(request))
      const organizationID = form.get('metadata[organization_id]') || ''
      const plan = form.get('metadata[plan_key]') || ''
      if (!organizationID || !plan || form.get('client_reference_id') !== organizationID || !request.headers['idempotency-key']) {
        respondJSON(response, 400, { error: { type: 'invalid_request_error', code: 'invalid_checkout_contract', message: 'Checkout metadata or idempotency is incomplete.' } })
        return
      }
      checkoutSequence += 1
      const sessionID = `cs_e2e_${checkoutSequence}`
      const customerID = `cus_e2e_${organizationID}`
      const subscriptionID = `sub_e2e_${organizationID}`
      subscriptions.set(subscriptionID, { customerID, organizationID, plan })
      recordCall({
        type: 'checkout',
        organizationID,
        plan,
        customerEmail: form.get('customer_email') || '',
        price: form.get('line_items[0][price]') || '',
        successURL: form.get('success_url') || '',
        cancelURL: form.get('cancel_url') || '',
        idempotencyPresent: Boolean(request.headers['idempotency-key'])
      })
      respondJSON(response, 200, { id: sessionID, url: `${origin}/checkout/${sessionID}`, expires_at: Math.floor(Date.now() / 1000) + 3600 })
      return
    }

    if (request.method === 'POST' && requestURL.pathname === '/v1/billing_portal/sessions') {
      const form = new URLSearchParams(await readBody(request))
      const customerID = form.get('customer') || ''
      if (!customerID || !request.headers['idempotency-key']) {
        respondJSON(response, 400, { error: { type: 'invalid_request_error', code: 'invalid_portal_contract', message: 'Portal customer or idempotency is incomplete.' } })
        return
      }
      portalSequence += 1
      const sessionID = `bps_e2e_${portalSequence}`
      recordCall({
        type: 'portal',
        customerID,
        returnURL: form.get('return_url') || '',
        idempotencyPresent: Boolean(request.headers['idempotency-key'])
      })
      respondJSON(response, 200, { id: sessionID, url: `${origin}/portal/${sessionID}` })
      return
    }

    if (request.method === 'GET' && requestURL.pathname.startsWith('/v1/subscriptions/')) {
      const subscriptionID = decodeURIComponent(requestURL.pathname.slice('/v1/subscriptions/'.length))
      const subscription = subscriptions.get(subscriptionID)
      if (!subscription) {
        respondJSON(response, 404, { error: { type: 'invalid_request_error', code: 'resource_missing', message: 'Subscription not found.' } })
        return
      }
      const now = Math.floor(Date.now() / 1000)
      recordCall({ type: 'subscription_reconcile', subscriptionID })
      respondJSON(response, 200, {
        id: subscriptionID,
        customer: subscription.customerID,
        status: 'active',
        current_period_start: now,
        current_period_end: now + 30 * 24 * 60 * 60,
        cancel_at_period_end: false,
        metadata: { organization_id: subscription.organizationID, plan_key: subscription.plan }
      })
      return
    }

    if (request.method === 'GET' && requestURL.pathname === '/v1/invoices') {
      recordCall({ type: 'invoice_reconcile', subscriptionID: requestURL.searchParams.get('subscription') || '' })
      respondJSON(response, 200, { data: [] })
      return
    }
  } catch (error) {
    respondJSON(response, 400, { error: { type: 'invalid_request_error', code: 'sandbox_request_invalid', message: error.message } })
    return
  }

  respondJSON(response, 404, { error: { type: 'invalid_request_error', code: 'not_found', message: 'Stripe sandbox route not found.' } })
})

server.listen(port, host)

function shutdown() {
  server.close(() => process.exit(0))
}

process.on('SIGINT', shutdown)
process.on('SIGTERM', shutdown)

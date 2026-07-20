import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { changePlan, createBillingPortalSession, createCheckoutSession, featureLabel, formatLimit, formatPrice, getEntitlements, listPlans, trialBanner } from '../lib/billing'
import { createIdempotencyKey } from '../lib/idempotency'
import { usePageTitle } from '../lib/use_page_title'

function UsageRow({ label, usage }) {
  if (!usage) {
    return null
  }
  return (
    <article className={usage.exceeded ? 'record-row record-row-alert' : 'record-row'} role="listitem">
      <div>
        <h3>{label}</h3>
        {usage.exceeded ? <p className="field-hint">Over plan limit — upgrade to add more.</p> : null}
      </div>
      <div>
        <p>{formatLimit(usage)}</p>
      </div>
    </article>
  )
}

export function SettingsBillingRoute() {
  const { session, canManageBilling, updateWorkspaceAccess } = useAuth()
  usePageTitle('Plan & Billing')
  const [entitlements, setEntitlements] = useState(null)
  const [plans, setPlans] = useState([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [pendingPlan, setPendingPlan] = useState('')

  useEffect(() => {
    const controller = new AbortController()
    async function load() {
      setIsLoading(true)
      try {
        const [nextEntitlements, nextPlans] = await Promise.all([
          getEntitlements({ signal: controller.signal }),
          listPlans({ signal: controller.signal })
        ])
        setEntitlements(nextEntitlements)
        setPlans(nextPlans)
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load plan details.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsLoading(false)
        }
      }
    }
    load()
    return () => {
      controller.abort()
    }
  }, [])

  const currentPlan = entitlements?.plan
  const activeFeatures = new Set(entitlements?.features || [])
  const subscription = entitlements?.subscription || null
  const billingProvider = subscription?.provider || 'fake'
  const checkoutPlans = new Set(subscription?.checkoutAvailablePlans || [])

  async function handleChangePlan(planKey) {
    setPendingPlan(planKey)
    setError('')
    try {
      const nextEntitlements = await changePlan(planKey)
      if (nextEntitlements) {
        setEntitlements(nextEntitlements)
        updateWorkspaceAccess({ state: 'writable' })
      }
    } catch (changeError) {
      if (!isAbortError(changeError)) {
        setError(changeError.message || 'Unable to change plan.')
      }
    } finally {
      setPendingPlan('')
    }
  }

  async function handleCheckout(planKey) {
    setPendingPlan(planKey)
    setError('')
    const storageKey = `open-crm-checkout-key:${planKey}`
    let idempotencyKey = window.sessionStorage.getItem(storageKey)
    if (!idempotencyKey) {
      idempotencyKey = createIdempotencyKey('checkout')
      window.sessionStorage.setItem(storageKey, idempotencyKey)
    }
    try {
      const session = await createCheckoutSession(planKey, idempotencyKey)
      if (!session?.url) throw new Error('Checkout did not return a secure destination.')
      window.sessionStorage.removeItem(storageKey)
      window.location.assign(session.url)
    } catch (checkoutError) {
      if (!isAbortError(checkoutError)) setError(checkoutError.message || 'Unable to open secure checkout.')
      setPendingPlan('')
    }
  }

  async function handlePortal() {
    setPendingPlan('portal')
    setError('')
    try {
      const session = await createBillingPortalSession()
      if (!session?.url) throw new Error('Billing portal did not return a secure destination.')
      window.location.assign(session.url)
    } catch (portalError) {
      if (!isAbortError(portalError)) setError(portalError.message || 'Unable to open the billing portal.')
      setPendingPlan('')
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Plan &amp; billing</h2>
              <p>Review the active plan and usage for {session?.organization?.name || 'your workspace'}.</p>
            </div>
            {currentPlan ? (
              <div>
                <span className="chip">{currentPlan.name} · {formatPrice(currentPlan)}</span>
              </div>
            ) : null}
          </div>
          {isLoading ? <p className="field-hint">Loading plan details...</p> : null}
          {error ? <InlineError message={error} /> : null}
          {!isLoading && entitlements?.subscription ? (() => {
            const banner = trialBanner(entitlements.subscription)
            return banner ? <div className="inline-note">{banner}</div> : null
          })() : null}
          {!isLoading && new URLSearchParams(window.location.search).get('checkout') === 'success' ? (
            <div className="inline-note" role="status">Checkout returned successfully. Access changes only after Open CRM receives and verifies Stripe’s subscription webhook; refresh if the new plan is still reconciling.</div>
          ) : null}
          {canManageBilling && subscription?.portalAvailable ? (
            <Button className="button-secondary" type="button" onClick={handlePortal} disabled={pendingPlan !== ''}>
              {pendingPlan === 'portal' ? 'Opening…' : 'Manage payment method, invoices, or cancellation'}
            </Button>
          ) : null}
          {!isLoading && entitlements ? (
            <div className="record-list" role="list" aria-label="Plan usage">
              <UsageRow label="Team seats" usage={entitlements.seats} />
              <UsageRow label="Contacts" usage={entitlements.contacts} />
              <UsageRow label="Deals" usage={entitlements.deals} />
            </div>
          ) : null}
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div>
            <h2>Compare plans</h2>
            <p>Upgrade to unlock more seats, higher limits, and additional features.</p>
          </div>
          <div className="record-list" role="list" aria-label="Available plans">
            {plans.map((plan) => (
              <article
                className={currentPlan && plan.key === currentPlan.key ? 'record-row record-row-active' : 'record-row'}
                key={plan.key}
                role="listitem"
              >
                <div>
                  <h3>{plan.name}{currentPlan && plan.key === currentPlan.key ? ' · Current plan' : ''}</h3>
                  <p className="field-hint">{plan.description}</p>
                  <p className="field-hint">
                    {(plan.features || []).map((feature) => featureLabel(feature)).join(' · ')}
                  </p>
                </div>
                <div>
                  <p>{formatPrice(plan)}</p>
                  {canManageBilling && currentPlan && plan.key !== currentPlan.key && billingProvider === 'fake' ? (
                    <Button
                      className="button-secondary"
                      type="button"
                      onClick={() => handleChangePlan(plan.key)}
                      disabled={pendingPlan !== ''}
                    >
                      {pendingPlan === plan.key ? 'Switching...' : 'Switch to this plan'}
                    </Button>
                  ) : null}
                  {canManageBilling && currentPlan && plan.key !== currentPlan.key && billingProvider === 'stripe' && checkoutPlans.has(plan.key) ? (
                    <Button className="button-secondary" type="button" onClick={() => handleCheckout(plan.key)} disabled={pendingPlan !== ''}>
                      {pendingPlan === plan.key ? 'Opening checkout…' : 'Continue to secure checkout'}
                    </Button>
                  ) : null}
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      {currentPlan ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>Included in {currentPlan.name}</h2>
              <p>Features available on your current plan.</p>
            </div>
            <div className="record-list" role="list" aria-label="Active features">
              {(currentPlan.features || []).map((feature) => (
                <article className="record-row" key={feature} role="listitem">
                  <div>
                    <h3>{featureLabel(feature)}</h3>
                  </div>
                  <div>
                    <p>{activeFeatures.has(feature) ? 'Active' : '—'}</p>
                  </div>
                </article>
              ))}
            </div>
          </div>
        </Card>
      ) : null}
    </section>
  )
}

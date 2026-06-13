import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { getEntitlements, listPlans, featureLabel, formatLimit, formatPrice } from '../lib/billing'
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
  const { session } = useAuth()
  usePageTitle('Plan & Billing')
  const [entitlements, setEntitlements] = useState(null)
  const [plans, setPlans] = useState([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

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

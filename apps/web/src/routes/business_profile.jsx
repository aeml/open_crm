import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { getBusinessProfile, updateBusinessProfile, upsertExchangeRate } from '../lib/business_profile'
import { usePageTitle } from '../lib/use_page_title'

const businessTypeOptions = [
  { value: 'general', label: 'General CRM' },
  { value: 'services', label: 'Services (Clients + Jobs)' },
  { value: 'product-sales', label: 'Product Sales (Accounts + Opportunities)' },
  { value: 'construction-services', label: 'Construction Services (Clients + Jobs)' }
]

function todayISODate() {
  return new Date().toISOString().slice(0, 10)
}

export function BusinessProfileRoute() {
  const { session, setBusinessProfile, canAdminister: canManageProfile } = useAuth()
  usePageTitle('Business Profile')
  const [profile, setProfile] = useState(null)
  const [businessType, setBusinessType] = useState('general')
  const [baseCurrency, setBaseCurrency] = useState('USD')
  const [rateForm, setRateForm] = useState({ quoteCurrency: '', rateToBase: '', effectiveDate: todayISODate(), source: 'manual' })
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [isSavingRate, setIsSavingRate] = useState(false)

  async function loadProfile({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextProfile = await getBusinessProfile({ signal })
      setProfile(nextProfile)
      setBusinessType(nextProfile?.businessType || 'general')
      setBaseCurrency(nextProfile?.baseCurrency || 'USD')
      setBusinessProfile(nextProfile)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load business profile.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()

    loadProfile({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [setBusinessProfile])

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManageProfile) {
      return
    }

    setIsSubmitting(true)
    setError('')
    try {
      const nextProfile = await updateBusinessProfile({ businessType, baseCurrency })
      setProfile(nextProfile)
      setBusinessProfile(nextProfile)
      setBaseCurrency(nextProfile?.baseCurrency || baseCurrency)
    } catch (submitError) {
      setError(submitError.message || 'Unable to update business profile.')
    } finally {
      setIsSubmitting(false)
    }
  }

  async function handleSaveRate(event) {
    event.preventDefault()
    if (!canManageProfile) {
      return
    }

    setIsSavingRate(true)
    setError('')
    try {
      const nextProfile = await upsertExchangeRate(rateForm.quoteCurrency, {
        rateToBase: rateForm.rateToBase,
        effectiveDate: rateForm.effectiveDate,
        source: rateForm.source
      })
      setProfile(nextProfile)
      setBusinessProfile(nextProfile)
      setBaseCurrency(nextProfile?.baseCurrency || baseCurrency)
      setRateForm({ quoteCurrency: '', rateToBase: '', effectiveDate: todayISODate(), source: 'manual' })
    } catch (rateError) {
      setError(rateError.message || 'Unable to save exchange rate.')
    } finally {
      setIsSavingRate(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Business profile</h2>
              <p>Shape the CRM around how {session?.organization?.name || 'your company'} actually works.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading business profile...</p> : null}
          {error ? (
            <InlineError message={error} onRetry={() => loadProfile()} retryLabel="Retry profile" />
          ) : null}
          <div className="record-list" role="list" aria-label="Adaptive labels preview">
            <article className="record-row" role="listitem">
              <div>
                <h3>{profile?.displayName || 'Loading profile'}</h3>
                <p>{profile?.businessType || businessType} · Base currency {profile?.baseCurrency || baseCurrency}</p>
              </div>
            </article>
            {Object.entries(profile?.labels || {}).map(([key, value]) => (
              <article className="record-row" key={key} role="listitem">
                <div>
                  <p>{key}</p>
                </div>
                <div>
                  <p>{value}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div>
            <h2>Adaptive settings</h2>
            <p>Choose the business type that drives modules, labels, and workflow defaults.</p>
          </div>
          <form className="auth-form" onSubmit={handleSubmit}>
            <Field label="Business type">
              <select className="text-input" value={businessType} onChange={(event) => setBusinessType(event.target.value)} disabled={!canManageProfile}>
                {businessTypeOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Base currency">
              <input className="text-input" maxLength={3} value={baseCurrency} onChange={(event) => setBaseCurrency(event.target.value.toUpperCase())} disabled={!canManageProfile} required />
            </Field>
            <Button type="submit" disabled={!canManageProfile || isSubmitting}>
              {isSubmitting ? 'Saving…' : 'Save business profile'}
            </Button>
          </form>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div>
            <h2>Currency rates</h2>
            <p>Set manual rates used to convert pipeline and forecast rollups into {profile?.baseCurrency || baseCurrency}. Individual records keep their original deal or catalog currency.</p>
          </div>
          <form className="auth-form" onSubmit={handleSaveRate}>
            <Field label="Quote currency">
              <input className="text-input" maxLength={3} value={rateForm.quoteCurrency} onChange={(event) => setRateForm((current) => ({ ...current, quoteCurrency: event.target.value.toUpperCase() }))} disabled={!canManageProfile} placeholder="EUR" required />
            </Field>
            <Field label={`Rate to ${profile?.baseCurrency || baseCurrency}`}>
              <input className="text-input" value={rateForm.rateToBase} onChange={(event) => setRateForm((current) => ({ ...current, rateToBase: event.target.value }))} disabled={!canManageProfile} placeholder="1.08000000" required />
            </Field>
            <Field label="Effective date">
              <input className="text-input" type="date" value={rateForm.effectiveDate} onChange={(event) => setRateForm((current) => ({ ...current, effectiveDate: event.target.value }))} disabled={!canManageProfile} required />
            </Field>
            <Field label="Source">
              <input className="text-input" maxLength={200} value={rateForm.source} onChange={(event) => setRateForm((current) => ({ ...current, source: event.target.value }))} disabled={!canManageProfile} placeholder="manual" />
            </Field>
            <Button type="submit" disabled={!canManageProfile || isSavingRate}>{isSavingRate ? 'Saving rate…' : 'Save exchange rate'}</Button>
          </form>
          <div className="record-list" role="list" aria-label="Exchange rates">
            {(profile?.exchangeRates || []).length === 0 ? (
              <p className="field-hint">No exchange rates yet. Add a rate before reporting on non-{profile?.baseCurrency || baseCurrency} deals.</p>
            ) : (profile?.exchangeRates || []).map((rate) => (
              <article className="record-row" key={rate.id} role="listitem">
                <div>
                  <p>{rate.quoteCurrency} to {rate.baseCurrency}</p>
                  <p className="field-hint">Effective {rate.effectiveDate} · {rate.source || 'manual'}</p>
                </div>
                <div>
                  <p>{rate.rateToBase}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
    </section>
  )
}

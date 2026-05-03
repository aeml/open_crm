import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { getBusinessProfile, updateBusinessProfile } from '../lib/business_profile'
import { usePageTitle } from '../lib/use_page_title'

const businessTypeOptions = [
  { value: 'general', label: 'General CRM' },
  { value: 'services', label: 'Services (Clients + Jobs)' },
  { value: 'product-sales', label: 'Product Sales (Accounts + Opportunities)' },
  { value: 'construction-services', label: 'Construction Services (Clients + Jobs)' }
]

export function BusinessProfileRoute() {
  const { session, setBusinessProfile } = useAuth()
  usePageTitle('Business Profile')
  const role = session?.membership?.role || ''
  const canManageProfile = useMemo(() => ['owner', 'admin'].includes(role), [role])
  const [profile, setProfile] = useState(null)
  const [businessType, setBusinessType] = useState('general')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSubmitting, setIsSubmitting] = useState(false)

  async function loadProfile({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextProfile = await getBusinessProfile({ signal })
      setProfile(nextProfile)
      setBusinessType(nextProfile?.businessType || 'general')
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
      const nextProfile = await updateBusinessProfile({ businessType })
      setProfile(nextProfile)
      setBusinessProfile(nextProfile)
    } catch (submitError) {
      setError(submitError.message || 'Unable to update business profile.')
    } finally {
      setIsSubmitting(false)
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
                <p>{profile?.businessType || businessType}</p>
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
            <Button type="submit" disabled={!canManageProfile || isSubmitting}>
              {isSubmitting ? 'Saving…' : 'Save business profile'}
            </Button>
          </form>
        </div>
      </Card>
    </section>
  )
}

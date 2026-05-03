import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listAuditEvents } from '../lib/audit'

const eventTypeOptions = [
  { value: '', label: 'All administrative events' },
  { value: 'user.invited', label: 'User invites' },
  { value: 'user.role_changed', label: 'Role changes' },
  { value: 'user.password_setup_completed', label: 'Password setup events' },
  { value: 'organization.profile_updated', label: 'Profile changes' }
]

function formatAuditTimestamp(value) {
  if (!value) {
    return 'Just now'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Just now' : date.toLocaleString()
}

export function SettingsAuditRoute() {
  const { session } = useAuth()
  const role = session?.membership?.role || ''
  const canReviewAudit = useMemo(() => ['owner', 'admin'].includes(role), [role])
  const [events, setEvents] = useState([])
  const [eventType, setEventType] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  async function loadEvents({ signal } = {}) {
    if (!canReviewAudit) {
      setError('Admin access required')
      setEvents([])
      setIsLoading(false)
      return
    }

    setIsLoading(true)
    try {
      const nextEvents = await listAuditEvents({ eventType, signal })
      setEvents(nextEvents)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load audit events.')
        setEvents([])
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadEvents({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [canReviewAudit, eventType])

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Admin audit trail</h2>
              <p>Review high-impact administrative changes for {session?.organization?.name || 'your workspace'}.</p>
            </div>
            <div>
              <Button className="button-secondary" type="button" onClick={() => loadEvents()} disabled={!canReviewAudit || isLoading}>Refresh audit</Button>
            </div>
          </div>
          <Field label="Audit event filter">
            <select className="text-input" value={eventType} onChange={(event) => setEventType(event.target.value)} disabled={!canReviewAudit}>
              {eventTypeOptions.map((option) => (
                <option key={option.value || 'all'} value={option.value}>{option.label}</option>
              ))}
            </select>
          </Field>
          {isLoading ? <p className="field-hint">Loading audit events...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Admin audit events">
            {!isLoading && events.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No audit events found.</p>
                  <p className="field-hint">Invites, role changes, password setup completions, and profile changes will appear here.</p>
                </div>
              </article>
            ) : events.map((event) => (
              <article className="record-row" key={event.id} role="listitem">
                <div>
                  <h3>{event.summary}</h3>
                  <p className="field-hint">{event.eventType} • {event.actorName || event.actorEmail || 'System'}</p>
                </div>
                <div>
                  <p>{formatAuditTimestamp(event.createdAt)}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div>
            <h2>Retention guidance</h2>
            <p>Keep audit events long enough to investigate administrative changes, then archive or purge them according to your operating policy.</p>
          </div>
          <div className="inline-note">
            <strong>Recommended baseline:</strong> retain admin audit events for at least 12 months and avoid storing password material, setup tokens, or other secrets in audit metadata.
          </div>
        </div>
      </Card>
    </section>
  )
}

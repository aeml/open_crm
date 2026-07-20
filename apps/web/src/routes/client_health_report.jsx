import { useEffect, useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { SavedViews } from '../components/ui/saved_views'
import { isAbortError } from '../lib/api'
import { getClientHealthReport } from '../lib/touchpoints'

const thresholds = [14, 30, 60, 90]

function healthClass(status) {
  return status === 'needs_attention' ? 'record-row-alert' : ''
}

function filtersFromSavedView(filters = {}) {
  const entityType = filters.entityType === 'contact' ? 'contact' : 'company'
  const status = ['all', 'healthy', 'watch', 'needs_attention'].includes(filters.status) ? filters.status : 'all'
  const staleDays = thresholds.includes(Number(filters.staleDays)) ? Number(filters.staleDays) : 30
  const ownerUserId = /^\d+$/.test(String(filters.ownerUserId || '')) ? String(filters.ownerUserId) : ''
  return { entityType, status, staleDays, ownerUserId }
}

export function ClientHealthReport({ onOpen, owners = [], canManage = true }) {
  const [draft, setDraft] = useState({ entityType: 'company', status: 'all', staleDays: 30, ownerUserId: '' })
  const [query, setQuery] = useState({ ...draft, run: 0 })
  const [report, setReport] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const controller = new AbortController()
    setIsLoading(true)
    getClientHealthReport({ ...query, signal: controller.signal })
      .then((result) => { setReport(result); setError('') })
      .catch((loadError) => { if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load client health.') })
      .finally(() => { if (!controller.signal.aborted) setIsLoading(false) })
    return () => controller.abort()
  }, [query])

  function applyFilters(event) {
    event?.preventDefault()
    setQuery({ ...draft, run: query.run + 1 })
  }

  function applySavedView(filters) {
    const next = filtersFromSavedView(filters)
    setDraft(next)
    setQuery({ ...next, run: query.run + 1 })
  }

  return (
    <Card>
      <div className="card-stack" aria-label="Client health">
        <div>
          <h2>Client health</h2>
          <p>Explainable attention signals from follow-up recency and open task deadlines.</p>
        </div>
        {report ? (
          <div className="button-row" aria-label="Client health totals">
            <span className="chip">Needs attention: {report.totals.needsAttention}</span>
            <span className="chip">Watch: {report.totals.watch}</span>
            <span className="chip">Healthy: {report.totals.healthy}</span>
          </div>
        ) : null}
        <SavedViews
          entityType="companies"
          canManage={canManage}
          viewScope="client-health"
          allowDefault={false}
          noun="segment"
          currentFilters={{ entityType: draft.entityType, status: draft.status, staleDays: String(draft.staleDays), ownerUserId: draft.ownerUserId }}
          onApply={applySavedView}
          defaultName="Client segment"
        />
        <form className="sales-report-filters" onSubmit={applyFilters}>
          <Field label="Client type">
            <select className="text-input" value={draft.entityType} onChange={(event) => setDraft({ ...draft, entityType: event.target.value })}>
              <option value="company">Organizations</option>
              <option value="contact">Individuals</option>
            </select>
          </Field>
          <Field label="Health">
            <select className="text-input" value={draft.status} onChange={(event) => setDraft({ ...draft, status: event.target.value })}>
              <option value="all">All health states</option>
              <option value="needs_attention">Needs attention</option>
              <option value="watch">Watch</option>
              <option value="healthy">Healthy</option>
            </select>
          </Field>
          <Field label="Stale after">
            <select className="text-input" value={draft.staleDays} onChange={(event) => setDraft({ ...draft, staleDays: Number(event.target.value) })}>
              {thresholds.map((days) => <option key={days} value={days}>{days} days</option>)}
            </select>
          </Field>
          <Field label="Owner">
            <select className="text-input" value={draft.ownerUserId} onChange={(event) => setDraft({ ...draft, ownerUserId: event.target.value })}>
              <option value="">Everyone</option>
              {owners.map((owner) => <option key={owner.id} value={owner.id}>{`${owner.firstName || ''} ${owner.lastName || ''}`.trim() || owner.email}{owner.status === 'disabled' ? ' (disabled)' : ''}</option>)}
            </select>
          </Field>
          <Button type="submit" disabled={isLoading}>{isLoading ? 'Checking...' : 'Apply health filters'}</Button>
        </form>
        {isLoading ? <p className="field-hint" role="status">Checking client health...</p> : null}
        {error ? <InlineError message={error} onRetry={applyFilters} retryLabel="Retry client health" /> : null}
        {!isLoading && !error && report ? (
          <>
            <p className="inline-note" role="status">{report.count} of {report.totals.total} {report.entityType === 'company' ? 'organization' : 'individual'} client{report.totals.total === 1 ? '' : 's'} match this health filter.</p>
            <div className="record-list" role="list" aria-label="Client health records">
              {report.records.length === 0 ? <article className="record-row" role="listitem"><p>No clients match these health filters.</p></article> : report.records.map((record) => (
                <article className={`record-row ${healthClass(record.healthStatus)}`} role="listitem" key={`${record.entityType}-${record.entityId}`}>
                  <div>
                    <button className="button button-ghost contact-link" type="button" onClick={() => onOpen(record)}>{record.label}</button>
                    {record.healthReasons.map((reason) => <p key={reason}>{reason}</p>)}
                    <p className="field-hint">{record.ownerUserName || 'Unassigned'} · {record.openTaskCount} open task{record.openTaskCount === 1 ? '' : 's'}</p>
                  </div>
                  <span className="chip">{record.healthLabel}</span>
                </article>
              ))}
            </div>
            {report.count > report.records.length ? <p className="field-hint">Showing the first {report.records.length} of {report.count}; resolve work, then rerun the filter.</p> : null}
            <details>
              <summary>How client health is calculated</summary>
              <ul>{report.semantics.map((rule) => <li key={rule}>{rule}</li>)}</ul>
            </details>
          </>
        ) : null}
      </div>
    </Card>
  )
}

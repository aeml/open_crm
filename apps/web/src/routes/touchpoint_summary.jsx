import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getTouchpointSummary } from '../lib/touchpoints'

const recordPaths = { contact: 'contacts', company: 'companies' }

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not recorded' : date.toLocaleString()
}

function sourceDescription(touch, entityType, entityId) {
  if (!touch || (touch.recordEntityType === entityType && touch.recordEntityId === entityId)) return null
  const path = recordPaths[touch.recordEntityType]
  return path ? <> via <Link to={`/${path}/${touch.recordEntityId}`}>{touch.recordLabel}</Link></> : ` via ${touch.recordLabel}`
}

export function TouchpointSummary({ entityType, entityId, refreshKey = '' }) {
  const [summary, setSummary] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [run, setRun] = useState(0)

  useEffect(() => {
    if (!entityId) return undefined
    const controller = new AbortController()
    setIsLoading(true)
    getTouchpointSummary(entityType, entityId, { signal: controller.signal })
      .then((result) => {
        setSummary(result)
        setError('')
      })
      .catch((loadError) => {
        if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load follow-up history.')
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false)
      })
    return () => controller.abort()
  }, [entityType, entityId, refreshKey, run])

  return (
    <Card className="touchpoint-summary-card">
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Follow-up</h3>
            <p>Traceable customer touches; routine record edits do not reset this clock.</p>
          </div>
          <div>
            {summary ? <span className="chip">{summary.healthLabel || (summary.isStale ? `Needs follow-up · ${summary.staleDays}+ days` : 'Current')}</span> : null}
            <Button className="button-secondary" type="button" onClick={() => setRun((current) => current + 1)} disabled={isLoading}>Refresh</Button>
          </div>
        </div>
        {isLoading ? <p className="field-hint" role="status">Loading follow-up history...</p> : null}
        {error ? <InlineError message={error} onRetry={() => setRun((current) => current + 1)} retryLabel="Retry follow-up history" /> : null}
        {!isLoading && !error && summary ? (
          <>
            {summary.lastTouch ? (
              <p><strong>Last touch:</strong> {summary.lastTouch.summary} · {formatTimestamp(summary.lastTouch.occurredAt)}{sourceDescription(summary.lastTouch, entityType, entityId)}</p>
            ) : (
              <p>No qualifying touch yet. Follow-up age starts from record creation on {formatTimestamp(summary.createdAt)}.</p>
            )}
            {summary.healthReasons.length > 0 ? <div aria-label="Health reasons">{summary.healthReasons.map((reason) => <p key={reason}>{reason}</p>)}</div> : null}
            <div className="button-row" aria-label="Account task health">
              <span className="chip">Open tasks: {summary.openTaskCount || 0}</span>
              <span className="chip">Overdue: {summary.overdueTaskCount || 0}</span>
              <span className="chip">Due soon: {summary.dueSoonTaskCount || 0}</span>
            </div>
            <div className="record-list" role="list" aria-label="Recent touchpoints">
              {summary.recent.length === 0 ? <article className="record-row" role="listitem"><p>No touchpoints recorded.</p></article> : summary.recent.slice(0, 5).map((touch) => (
                <article className="record-row" role="listitem" key={`${touch.sourceType}-${touch.sourceId}-${touch.action}`}>
                  <div>
                    <h4>{touch.summary}</h4>
                    <p className="field-hint">{formatTimestamp(touch.occurredAt)}{sourceDescription(touch, entityType, entityId)}</p>
                  </div>
                  <span className="chip">{touch.action}</span>
                </article>
              ))}
            </div>
            <details>
              <summary>How touchpoints are calculated</summary>
              <ul>{summary.semantics.map((rule) => <li key={rule}>{rule}</li>)}</ul>
            </details>
            {summary.healthSemantics.length > 0 ? <details><summary>How health is calculated</summary><ul>{summary.healthSemantics.map((rule) => <li key={rule}>{rule}</li>)}</ul></details> : null}
          </>
        ) : null}
      </div>
    </Card>
  )
}

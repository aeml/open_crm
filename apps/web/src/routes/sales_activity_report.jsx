import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getSalesActivityReport } from '../lib/reports'
import { listOrganizationUsers } from '../lib/users'

function dateInputValue(date) {
  return date.toISOString().slice(0, 10)
}

function defaultFilters() {
  const to = new Date()
  const from = new Date(to)
  from.setUTCDate(from.getUTCDate() - 29)
  return { from: dateInputValue(from), to: dateInputValue(to), ownerUserId: '' }
}

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not recorded' : date.toLocaleString()
}

function metricValue(value) {
  return Number(value || 0).toLocaleString()
}

function baseAmount(value, currency) {
  return `${Number(value || 0).toLocaleString(undefined, { minimumFractionDigits: 2 })} ${currency || 'base'}`
}

function CoverageNote({ complete, label, startedAt, missing }) {
  return (
    <p className={complete ? 'inline-note' : 'inline-note sales-report-coverage-warning'} role="status">
      {label}: {complete ? 'complete' : 'partial'} since {formatTimestamp(startedAt)}.{complete ? '' : ` ${missing}`}
    </p>
  )
}

function outcomeLabel(value) {
  if (value === 'won') return 'won'
  if (value === 'lost') return 'lost'
  return 'open'
}

function eventSummary(event) {
  if (event.eventType === 'created') return `Created in ${event.toPipelineName} / ${event.toStageName}`
  return `${event.fromStageName} → ${event.toStageName}`
}

export function SalesActivityReport() {
  const [draft, setDraft] = useState(defaultFilters)
  const [query, setQuery] = useState(() => ({ ...defaultFilters(), run: 0 }))
  const [users, setUsers] = useState([])
  const [usersError, setUsersError] = useState('')
  const [usersRun, setUsersRun] = useState(0)
  const [report, setReport] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const controller = new AbortController()
    listOrganizationUsers({ includeDisabled: true, signal: controller.signal })
      .then((result) => {
        setUsers(result)
        setUsersError('')
      })
      .catch((loadError) => {
        if (!isAbortError(loadError)) setUsersError(loadError.message || 'Unable to load teammates for reporting.')
      })
    return () => controller.abort()
  }, [usersRun])

  useEffect(() => {
    const controller = new AbortController()
    setIsLoading(true)
    getSalesActivityReport({ from: query.from, to: query.to, ownerUserId: query.ownerUserId, signal: controller.signal })
      .then((result) => {
        setReport(result)
        setError('')
      })
      .catch((loadError) => {
        if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load sales activity reporting.')
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false)
      })
    return () => controller.abort()
  }, [query])

  function runReport(event) {
    event?.preventDefault()
    setQuery({ ...draft, run: query.run + 1 })
  }

  const totals = report?.totals || {}
  const baseCurrency = report?.baseCurrency || ''
  const wonRevenue = baseAmount(totals.wonRevenueBase, baseCurrency)
  const metrics = [
    ['Deals created', totals.dealsCreated],
    ['Stage moves', totals.stageMoves],
    ['Won outcomes', totals.dealsWon],
    ['Lost outcomes', totals.dealsLost],
    ['Outcome win rate', totals.winRatePercent ? `${totals.winRatePercent}%` : '—'],
    [`Won revenue${baseCurrency ? ` (${baseCurrency})` : ''}`, wonRevenue],
    ['Notes added', totals.notesAdded],
    ['Tasks created', totals.tasksCreated],
    ['Tasks completed', totals.tasksCompleted]
  ]

  return (
    <Card className="sales-activity-card">
      <div className="card-stack">
        <div>
          <h2>Sales activity</h2>
          <p>Live, tenant-scoped deal progress and follow-up work. Date windows use UTC calendar days.</p>
        </div>
        <form className="sales-report-filters" onSubmit={runReport}>
          <Field label="From date (UTC)">
            <input className="text-input" type="date" value={draft.from} onChange={(event) => setDraft({ ...draft, from: event.target.value })} required />
          </Field>
          <Field label="To date (UTC)">
            <input className="text-input" type="date" value={draft.to} onChange={(event) => setDraft({ ...draft, to: event.target.value })} required />
          </Field>
          <Field label="Teammate">
            <select className="text-input" value={draft.ownerUserId} onChange={(event) => setDraft({ ...draft, ownerUserId: event.target.value })}>
              <option value="">Everyone</option>
              {users.map((user) => <option key={user.id} value={user.id}>{user.firstName} {user.lastName}{user.status === 'disabled' ? ' (disabled)' : ''}</option>)}
            </select>
          </Field>
          <Button type="submit" disabled={isLoading}>{isLoading ? 'Running...' : 'Run report'}</Button>
        </form>
        {usersError ? <InlineError message={usersError} onRetry={() => setUsersRun((current) => current + 1)} retryLabel="Retry teammates" /> : null}
        {error ? <InlineError message={error} onRetry={runReport} retryLabel="Retry sales report" /> : null}
        {isLoading ? <p className="field-hint" role="status">Running sales activity report...</p> : null}
        {!isLoading && !error && report ? (
          <>
            <CoverageNote complete={report.historyComplete} label="Event history" startedAt={report.coverageStartedAt} missing="Earlier events are not inferred." />
            <CoverageNote complete={report.closeReasonHistoryComplete} label="Close-reason history" startedAt={report.closeReasonCoverageStartedAt} missing="Earlier reasons are not captured." />
            <CoverageNote complete={report.revenueHistoryComplete} label="Won-revenue history" startedAt={report.revenueTrackingStartedAt} missing="Earlier value and FX are not inferred." />
            <div className="sales-report-metrics" role="list" aria-label="Sales activity totals">
              {metrics.map(([label, value]) => (
                <div className="sales-report-metric" role="listitem" key={label}>
                  <p className="metric-label">{label}</p>
                  <p className="metric-value">{typeof value === 'number' || value == null ? metricValue(value) : value}</p>
                </div>
              ))}
            </div>
            <p className="field-hint">{report.outcomeMeaning} {report.revenueMeaning}</p>
            <p className={(totals.wonRevenueMissingValue || totals.wonRevenueMissingRate) ? 'inline-note sales-report-coverage-warning' : 'inline-note'} role="status">
              Revenue inputs: {metricValue(totals.wonRevenueCaptured)} backed, {metricValue(totals.wonRevenueMissingValue)} missing value/currency, {metricValue(totals.wonRevenueMissingRate)} missing event-time FX.
            </p>
            <div className="card-stack">
              <div>
                <h3>Win/loss reasons</h3>
                <p className="field-hint">{report.closeReasonMeaning}</p>
              </div>
              <div className="record-list" role="list" aria-label="Win and loss reasons">
                {(report.closeReasons || []).length === 0 ? <article className="record-row" role="listitem"><p>No closed outcomes in this window.</p></article> : report.closeReasons.map((reason) => (
                  <article className="record-row" role="listitem" key={`${reason.outcome}-${reason.reasonCode}`}>
                    <div><h4>{reason.reasonLabel}</h4><p>{reason.outcome === 'won' ? 'Won' : 'Lost'} outcomes</p></div>
                    <span className="chip">{metricValue(reason.count)}</span>
                  </article>
                ))}
              </div>
            </div>
            <div className="card-stack">
              <div>
                <h3>By teammate</h3>
                <p className="field-hint">{report.ownerFilterMeaning}</p>
              </div>
              <div className="record-list" role="list" aria-label="Sales activity by teammate">
                {report.owners.length === 0 ? <article className="record-row" role="listitem"><p>No teammate activity in this window.</p></article> : report.owners.map((owner) => (
                  <article className="record-row" role="listitem" key={owner.userId}>
                    <div>
                      <h4>{owner.userName || owner.email}</h4>
                      <p>{owner.dealsCreated} created · {owner.stageMoves} stage moves · {owner.dealsWon} won · {owner.dealsLost} lost</p>
                      <p>{baseAmount(owner.wonRevenueBase, baseCurrency)} won revenue · {owner.wonRevenueCaptured} backed</p>
                      <p>{owner.notesAdded} notes · {owner.tasksCreated} tasks created · {owner.tasksCompleted} completed</p>
                    </div>
                    <span className="chip">{owner.status || 'active'}</span>
                  </article>
                ))}
              </div>
            </div>
            <div className="card-stack">
              <div>
                <h3>Stage movement</h3>
                <p className="field-hint">{report.stageConversionMeaning}</p>
              </div>
              <div className="record-list" role="list" aria-label="Stage movement report">
                {report.stages.length === 0 ? <article className="record-row" role="listitem"><p>No stage activity in this window.</p></article> : report.stages.map((stage) => (
                  <article className="record-row" role="listitem" key={`${stage.pipelineId}-${stage.stageId}`}>
                    <div>
                      <h4>{stage.pipelineName} / {stage.stageName}</h4>
                      <p>{stage.entries} entries · {stage.exits} exits · {stage.forwardExits} forward · {stage.wonExits} won · {stage.lostExits} lost</p>
                    </div>
                    <span className="chip">{stage.forwardExitRatePercent ? `${stage.forwardExitRatePercent}% forward exits` : 'No exits'}</span>
                  </article>
                ))}
              </div>
            </div>
            <div className="card-stack">
              <div>
                <h3>Recent deal events</h3>
                <p className="field-hint">Up to 50 snapshot-backed events in the selected window.</p>
              </div>
              <div className="record-list" role="list" aria-label="Recent deal events">
                {report.dealEvents.length === 0 ? <article className="record-row" role="listitem"><p>No deal events in this window.</p></article> : report.dealEvents.map((event) => (
                  <article className="record-row" role="listitem" key={event.id}>
                    <div>
                      <h4><Link to={`/deals/${event.dealId}`}>{event.dealName}</Link></h4>
                      <p>{eventSummary(event)} · now {outcomeLabel(event.toStageOutcome)}</p>
                      {event.closeReasonLabel ? <p>{event.closeReasonLabel}{event.closeNotes ? ` · ${event.closeNotes}` : ''}</p> : null}
                      <p>{event.actorName || 'Unknown actor'} · owner {event.ownerName || 'Unassigned'} · {formatTimestamp(event.occurredAt)}</p>
                    </div>
                  </article>
                ))}
              </div>
            </div>
          </>
        ) : null}
      </div>
    </Card>
  )
}

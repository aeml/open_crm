import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getClientActivityReport } from '../lib/touchpoints'
import { listOrganizationUsers } from '../lib/users'

const recordPaths = { contact: 'contacts', company: 'companies' }

function dateInputValue(date) {
  return date.toISOString().slice(0, 10)
}

function defaultFilters() {
  const to = new Date()
  const from = new Date(to)
  from.setUTCDate(from.getUTCDate() - 29)
  return { entityType: 'company', from: dateInputValue(from), to: dateInputValue(to), activity: 'all', ownerUserId: '' }
}

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not recorded' : date.toLocaleString()
}

function metricValue(value) {
  return Number(value || 0).toLocaleString()
}

function reportMatchesQuery(report, query) {
  return report.entityType === query.entityType && report.fromDate === query.from && report.toDate === query.to && report.activity === query.activity && Number(report.ownerUserId || 0) === Number(query.ownerUserId || 0)
}

export function ClientActivityReport() {
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
      .then((result) => { setUsers(result); setUsersError('') })
      .catch((loadError) => { if (!isAbortError(loadError)) setUsersError(loadError.message || 'Unable to load teammates for client activity reporting.') })
    return () => controller.abort()
  }, [usersRun])

  useEffect(() => {
    const controller = new AbortController()
    setIsLoading(true)
    getClientActivityReport({ ...query, signal: controller.signal })
      .then((result) => {
        if (!reportMatchesQuery(result, query)) throw new Error('The client activity report returned a different filter window.')
        setReport(result)
        setError('')
      })
      .catch((loadError) => { if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load client activity.') })
      .finally(() => { if (!controller.signal.aborted) setIsLoading(false) })
    return () => controller.abort()
  }, [query])

  function runReport(event) {
    event?.preventDefault()
    setQuery({ ...draft, run: query.run + 1 })
  }

  const totals = report?.totals || {}
  const metrics = [
    ['Clients', totals.totalClients],
    ['With activity', totals.clientsWithActivity],
    ['Without activity', totals.clientsWithoutActivity],
    ['Qualifying touches', totals.qualifyingTouches],
    ['Notes added', totals.notesAdded],
    ['Tasks completed', totals.tasksCompleted]
  ]

  return (
    <Card className="client-activity-report-card">
      <div className="card-stack" aria-label="Client period activity">
        <div>
          <h2>Client activity</h2>
          <p>Current customers and their qualifying post-sale work during an inclusive UTC period. Clients without activity appear first.</p>
        </div>
        <form className="sales-report-filters" onSubmit={runReport}>
          <Field label="Client type">
            <select className="text-input" value={draft.entityType} onChange={(event) => setDraft({ ...draft, entityType: event.target.value })}>
              <option value="company">Organizations</option>
              <option value="contact">Individuals</option>
            </select>
          </Field>
          <Field label="From date (UTC)">
            <input className="text-input" type="date" value={draft.from} onChange={(event) => setDraft({ ...draft, from: event.target.value })} required />
          </Field>
          <Field label="To date (UTC)">
            <input className="text-input" type="date" value={draft.to} onChange={(event) => setDraft({ ...draft, to: event.target.value })} required />
          </Field>
          <Field label="Activity">
            <select className="text-input" value={draft.activity} onChange={(event) => setDraft({ ...draft, activity: event.target.value })}>
              <option value="all">All clients</option>
              <option value="without_activity">Without activity</option>
              <option value="with_activity">With activity</option>
            </select>
          </Field>
          <Field label="Current owner">
            <select className="text-input" value={draft.ownerUserId} onChange={(event) => setDraft({ ...draft, ownerUserId: event.target.value })}>
              <option value="">Everyone</option>
              {users.map((user) => <option key={user.id} value={user.id}>{user.firstName} {user.lastName}{user.status === 'disabled' ? ' (disabled)' : ''}</option>)}
            </select>
          </Field>
          <Button type="submit" disabled={isLoading}>{isLoading ? 'Running...' : 'Run client activity'}</Button>
        </form>
        {usersError ? <InlineError message={usersError} onRetry={() => setUsersRun((current) => current + 1)} retryLabel="Retry teammates" /> : null}
        {error ? <InlineError message={error} onRetry={runReport} retryLabel="Retry client activity" /> : null}
        {isLoading ? <p className="field-hint" role="status">Running client activity report...</p> : null}
        {!isLoading && !error && report ? (
          <>
            <p className="inline-note" role="status">{report.count} client{report.count === 1 ? '' : 's'} match these filters. Generated {formatTimestamp(report.generatedAt)}.</p>
            <div className="sales-report-metrics" role="list" aria-label="Client activity totals">
              {metrics.map(([label, value]) => (
                <div className="sales-report-metric" role="listitem" key={label}>
                  <p className="metric-label">{label}</p>
                  <p className="metric-value">{metricValue(value)}</p>
                </div>
              ))}
            </div>
            <div className="table-scroll" tabIndex="0" role="region" aria-label="Client activity records">
              <table className="data-table">
                <caption>Client activity from {report.fromDate} through {report.toDate} UTC</caption>
                <thead><tr><th scope="col">Client</th><th scope="col">Current owner</th><th scope="col">Touches</th><th scope="col">Notes</th><th scope="col">Tasks completed</th><th scope="col">Active days</th><th scope="col">Latest period touch</th></tr></thead>
                <tbody>
                  {report.records.length === 0 ? <tr><td colSpan="7">No clients match these period filters.</td></tr> : report.records.map((record) => (
                    <tr className={record.qualifyingTouches === 0 ? 'record-row-alert' : ''} key={`${record.entityType}-${record.entityId}`}>
                      <th scope="row"><Link to={`/${recordPaths[record.entityType]}/${record.entityId}`}>{record.label}</Link></th>
                      <td>{record.ownerUserName || 'Unassigned'}</td>
                      <td>{metricValue(record.qualifyingTouches)}</td>
                      <td>{metricValue(record.notesAdded)}</td>
                      <td>{metricValue(record.tasksCompleted)}</td>
                      <td>{metricValue(record.activeDays)}</td>
                      <td>{record.lastTouchInPeriod ? <><Link to={`/${recordPaths[record.lastTouchInPeriod.recordEntityType]}/${record.lastTouchInPeriod.recordEntityId}`}>{record.lastTouchInPeriod.summary}</Link> · {formatTimestamp(record.lastTouchInPeriod.occurredAt)}</> : 'No qualifying touch in period'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {report.count > report.records.length ? <p className="field-hint">Showing the first {report.records.length} of {report.count}; narrow the owner or activity filter to inspect another bounded set.</p> : null}
            <details>
              <summary>How client activity is calculated</summary>
              <ul>{report.semantics.map((rule) => <li key={rule}>{rule}</li>)}</ul>
            </details>
          </>
        ) : null}
      </div>
    </Card>
  )
}

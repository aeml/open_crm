import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getFollowUpReport } from '../lib/touchpoints'
import { listOrganizationUsers } from '../lib/users'

const thresholdOptions = [14, 30, 60, 90]
const recordPaths = { contact: 'contacts', company: 'companies' }

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not recorded' : date.toLocaleString()
}

export function FollowUpReport() {
  const [draft, setDraft] = useState({ entityType: 'contact', staleDays: 30, ownerUserId: '' })
  const [query, setQuery] = useState({ entityType: 'contact', staleDays: 30, ownerUserId: '', run: 0 })
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
        if (!isAbortError(loadError)) setUsersError(loadError.message || 'Unable to load teammates for follow-up reporting.')
      })
    return () => controller.abort()
  }, [usersRun])

  useEffect(() => {
    const controller = new AbortController()
    setIsLoading(true)
    getFollowUpReport({ ...query, signal: controller.signal })
      .then((result) => {
        setReport(result)
        setError('')
      })
      .catch((loadError) => {
        if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load the follow-up queue.')
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

  return (
    <Card className="follow-up-report-card">
      <div className="card-stack">
        <div>
          <h2>Follow-up queue</h2>
          <p>Contacts and clients whose latest qualifying customer touch—or creation date when no touch exists—is older than the selected threshold.</p>
        </div>
        <form className="sales-report-filters" onSubmit={runReport}>
          <Field label="Record type">
            <select className="text-input" value={draft.entityType} onChange={(event) => setDraft({ ...draft, entityType: event.target.value })}>
              <option value="contact">Contacts</option>
              <option value="company">Clients</option>
            </select>
          </Field>
          <Field label="No touch for">
            <select className="text-input" value={draft.staleDays} onChange={(event) => setDraft({ ...draft, staleDays: Number(event.target.value) })}>
              {thresholdOptions.map((days) => <option key={days} value={days}>{days}+ days</option>)}
            </select>
          </Field>
          <Field label="Owner">
            <select className="text-input" value={draft.ownerUserId} onChange={(event) => setDraft({ ...draft, ownerUserId: event.target.value })}>
              <option value="">Everyone</option>
              {users.map((user) => <option key={user.id} value={user.id}>{user.firstName} {user.lastName}{user.status === 'disabled' ? ' (disabled)' : ''}</option>)}
            </select>
          </Field>
          <Button type="submit" disabled={isLoading}>{isLoading ? 'Running...' : 'Run report'}</Button>
        </form>
        {usersError ? <InlineError message={usersError} onRetry={() => setUsersRun((current) => current + 1)} retryLabel="Retry teammates" /> : null}
        {error ? <InlineError message={error} onRetry={runReport} retryLabel="Retry follow-up report" /> : null}
        {isLoading ? <p className="field-hint" role="status">Finding records that need follow-up...</p> : null}
        {!isLoading && !error && report ? (
          <>
            <p className="inline-note" role="status">{report.count} {report.entityType === 'company' ? 'client' : 'contact'} record{report.count === 1 ? '' : 's'} need follow-up. Generated {formatTimestamp(report.generatedAt)}.</p>
            <div className="record-list" role="list" aria-label="Follow-up queue records">
              {report.records.length === 0 ? <article className="record-row" role="listitem"><p>No records need follow-up for these filters.</p></article> : report.records.map((record) => (
                <article className="record-row record-row-alert" role="listitem" key={`${record.entityType}-${record.entityId}`}>
                  <div>
                    <h3><Link to={`/${recordPaths[record.entityType]}/${record.entityId}`}>{record.label}</Link></h3>
                    <p>{record.lastTouch ? `${record.lastTouch.summary} · ${formatTimestamp(record.lastTouch.occurredAt)}` : `No qualifying touch · created ${formatTimestamp(record.createdAt)}`}</p>
                    {record.lastTouch && (record.lastTouch.recordEntityType !== record.entityType || record.lastTouch.recordEntityId !== record.entityId) ? <p className="field-hint">Source: <Link to={`/${recordPaths[record.lastTouch.recordEntityType]}/${record.lastTouch.recordEntityId}`}>{record.lastTouch.recordLabel}</Link></p> : null}
                  </div>
                  <div>
                    <span className="chip">{record.daysSinceReference} days</span>
                    <p className="field-hint">{record.ownerUserName || 'Unassigned'}</p>
                  </div>
                </article>
              ))}
            </div>
            {report.count > report.records.length ? <p className="field-hint">Showing the first {report.records.length} of {report.count}; follow up, then rerun for the next records.</p> : null}
            <details>
              <summary>How the follow-up queue is calculated</summary>
              <ul>{report.semantics.map((rule) => <li key={rule}>{rule}</li>)}</ul>
            </details>
          </>
        ) : null}
      </div>
    </Card>
  )
}

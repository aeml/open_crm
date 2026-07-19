import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listArchivedRecords, restoreArchivedRecord } from '../lib/archives'
import { usePageTitle } from '../lib/use_page_title'

const entityOptions = [
  { value: '', label: 'All record types' },
  { value: 'contact', label: 'Contacts' },
  { value: 'company', label: 'Companies' },
  { value: 'deal', label: 'Deals' },
  { value: 'task', label: 'Tasks' }
]

const recordPaths = {
  contact: 'contacts',
  company: 'companies',
  deal: 'deals',
  task: 'tasks'
}

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Unknown date' : date.toLocaleString()
}

function entityLabel(entityType) {
  return entityOptions.find((option) => option.value === entityType)?.label?.replace(/s$/, '') || 'Record'
}

export function SettingsArchivedRecordsRoute() {
  const { session } = useAuth()
  usePageTitle('Archived Records')
  const canRestore = ['owner', 'admin', 'member'].includes(session?.membership?.role || '')
  const [records, setRecords] = useState([])
  const [entityType, setEntityType] = useState('')
  const [search, setSearch] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [restoredRecord, setRestoredRecord] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [restoringKey, setRestoringKey] = useState('')

  async function load({ signal } = {}) {
    setIsLoading(true)
    try {
      setRecords(await listArchivedRecords({ entityType, search: appliedSearch, signal }))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load archived records.')
        setRecords([])
      }
    } finally {
      if (!signal?.aborted) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [entityType, appliedSearch])

  async function handleRestore(record) {
    if (!window.confirm(`Restore ${record.label}? It will return to active ${record.entityType} views.`)) return
    const key = `${record.entityType}-${record.entityId}`
    setRestoringKey(key)
    setError('')
    setNotice('')
    try {
      await restoreArchivedRecord(record.entityType, record.entityId)
      setRecords((current) => current.filter((item) => `${item.entityType}-${item.entityId}` !== key))
      setRestoredRecord(record)
      setNotice(`${record.label} was restored.`)
    } catch (restoreError) {
      setError(restoreError.message || 'Unable to restore archived record.')
    } finally {
      setRestoringKey('')
    }
  }

  function submitSearch(event) {
    event.preventDefault()
    setAppliedSearch(search.trim())
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Archived records</h2>
              <p>Find and restore contacts, companies, deals, and tasks removed from normal workspace views.</p>
            </div>
            <Button className="button-secondary" type="button" onClick={() => load()} disabled={isLoading}>Refresh</Button>
          </div>

          <form className="form-grid form-grid-two" onSubmit={submitSearch}>
            <Field label="Record type">
              <select className="text-input" value={entityType} onChange={(event) => setEntityType(event.target.value)}>
                {entityOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Search archived records">
              <div className="input-action-row">
                <input className="text-input" value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Name, title, or exact ID" />
                <Button className="button-secondary" type="submit">Search</Button>
              </div>
            </Field>
          </form>

          {!canRestore ? <div className="inline-note">Your view-only role can inspect archive history. A workspace member or admin can restore records.</div> : null}
          {isLoading ? <p className="field-hint" role="status">Loading archived records...</p> : null}
          {error ? <InlineError message={error} /> : null}
          {notice ? (
            <div className="inline-note" role="status">
              {notice}{restoredRecord && recordPaths[restoredRecord.entityType] ? <> <Link to={`/${recordPaths[restoredRecord.entityType]}/${restoredRecord.entityId}`}>Open restored record</Link>.</> : null}
            </div>
          ) : null}

          <div className="record-list" role="list" aria-label="Archived records">
            {!isLoading && records.length === 0 ? (
              <article className="record-row" role="listitem">
                <div><p>No archived records match these filters.</p><p className="field-hint">Archived records remain retained until workspace offboarding or an explicit future retention policy applies.</p></div>
              </article>
            ) : records.map((record) => {
              const key = `${record.entityType}-${record.entityId}`
              return (
                <article className={record.restoreBlockedReason ? 'record-row record-row-alert' : 'record-row'} key={key} role="listitem">
                  <div>
                    <h3>{record.label}</h3>
                    <p className="field-hint">{entityLabel(record.entityType)} #{record.entityId} · archived {formatTimestamp(record.archivedAt)}{record.ownerName ? ` · owner ${record.ownerName}` : ''}</p>
                    {record.restoreBlockedReason ? <p>{record.restoreBlockedReason}</p> : null}
                  </div>
                  {canRestore && !record.restoreBlockedReason ? (
                    <Button className="button-secondary" type="button" onClick={() => handleRestore(record)} disabled={restoringKey === key}>
                      {restoringKey === key ? 'Restoring...' : 'Restore'}
                    </Button>
                  ) : null}
                </article>
              )
            })}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div><h2>Recovery rules</h2><p>Related notes, tasks, and activity stay attached while a record is archived. Deals and tasks can be restored only after their linked records are active.</p></div>
          <div className="inline-note"><strong>Duplicate merges are permanent:</strong> consumed source records remain visible as history but cannot be restored because their relationships and selected values now belong to the surviving record.</div>
        </div>
      </Card>
    </section>
  )
}

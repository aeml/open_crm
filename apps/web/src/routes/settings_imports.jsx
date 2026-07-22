import { useEffect, useState } from 'react'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { executeImport, importErrorsURL, listImports, previewImport, rollbackImport } from '../lib/imports'
import { createIdempotencyKey } from '../lib/idempotency'
import { usePageTitle } from '../lib/use_page_title'

function formatTimestamp(value) {
  if (!value) return 'Not finished'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not recorded' : date.toLocaleString()
}

function statusLabel(status) {
  return String(status || '').replaceAll('_', ' ')
}

function newIdempotencyKey() {
  return createIdempotencyKey('import')
}

function mappingHasErrors(preview) {
  return (preview?.mappingErrors || []).length > 0
}

export function SettingsImportsRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Data imports')
  const [entityType, setEntityType] = useState('contacts')
  const [file, setFile] = useState(null)
  const [mapping, setMapping] = useState({})
  const [mappingDirty, setMappingDirty] = useState(false)
  const [preview, setPreview] = useState(null)
  const [batches, setBatches] = useState([])
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [isPreviewing, setIsPreviewing] = useState(false)
  const [isImporting, setIsImporting] = useState(false)
  const [rollingBackId, setRollingBackId] = useState(0)
  const [idempotencyKey, setIdempotencyKey] = useState(newIdempotencyKey)

  async function loadHistory({ signal } = {}) {
    if (!canManage) {
      setBatches([])
      return
    }
    try {
      setBatches(await listImports({ signal }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load import history.')
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadHistory({ signal: controller.signal })
    return () => controller.abort()
  }, [canManage])

  useEffect(() => {
    const active = batches.some((batch) => batch.status === 'processing' || ['pending', 'running', 'retryable'].includes(batch.jobStatus))
    if (!active) return undefined
    const timer = window.setTimeout(() => loadHistory(), 3000)
    return () => window.clearTimeout(timer)
  }, [batches])

  function selectFile(nextFile) {
    setFile(nextFile || null)
    setPreview(null)
    setMapping({})
    setMappingDirty(false)
    setNotice('')
    setIdempotencyKey(newIdempotencyKey())
  }

  function selectEntity(nextEntityType) {
    setEntityType(nextEntityType)
    setPreview(null)
    setMapping({})
    setMappingDirty(false)
    setNotice('')
    setIdempotencyKey(newIdempotencyKey())
  }

  async function handlePreview() {
    if (!file) {
      setError('Choose a CSV file first.')
      return
    }
    setIsPreviewing(true)
    setError('')
    setNotice('')
    try {
      const result = await previewImport(file, entityType, Object.keys(mapping).length > 0 ? mapping : undefined)
      setPreview(result)
      setMapping(result?.mapping || {})
      setMappingDirty(false)
    } catch (previewError) {
      setError(previewError.message || 'Unable to preview CSV import.')
    } finally {
      setIsPreviewing(false)
    }
  }

  async function handleImport() {
    if (!file || !preview || mappingDirty || mappingHasErrors(preview)) {
      setError('Preview the file and fix required column mappings before importing.')
      return
    }
    if (preview.summary?.errorRows > 0 && !window.confirm(`${preview.summary.errorRows} rows have validation errors and will be skipped. Import the valid rows?`)) {
      return
    }
    setIsImporting(true)
    setError('')
    setNotice('')
    try {
      const batch = await executeImport(file, entityType, mapping, idempotencyKey)
      setNotice(batch.replayed ? `Import ${batch.id} already exists; follow its result below.` : `Import queued: 0 / ${batch.totalRows} processed. You can leave this page.`)
      setFile(null)
      setPreview(null)
      setMapping({})
      setMappingDirty(false)
      setIdempotencyKey(newIdempotencyKey())
      await loadHistory()
    } catch (importError) {
      setError(importError.message || 'Unable to import CSV data. Retry with the same file; the operation is idempotent.')
    } finally {
      setIsImporting(false)
    }
  }

  async function handleRollback(batch) {
    if (!window.confirm(`Archive the ${batch.successRows} records created by this import? Records changed since import will be left active for safety.`)) {
      return
    }
    setRollingBackId(batch.id)
    setError('')
    setNotice('')
    try {
      const result = await rollbackImport(batch.id)
      setNotice(`Rollback finished: ${result.rolledBackRows} archived, ${result.rollbackSkippedRows} changed records left active.`)
      await loadHistory()
    } catch (rollbackError) {
      setError(rollbackError.message || 'Unable to roll back import.')
    } finally {
      setRollingBackId(0)
    }
  }

  async function handleResume(batch) {
    if (!file) {
      setError('Choose the original CSV file before resuming this import.')
      return
    }
    setIsImporting(true)
    setError('')
    setNotice('')
    try {
      const result = await executeImport(file, batch.entityType, batch.mapping, batch.idempotencyKey)
      setNotice(`Import ${result.id} already exists; follow its result below.`)
      await loadHistory()
    } catch (resumeError) {
      setError(resumeError.message || 'Unable to resume import. Confirm this is the exact original file.')
    } finally {
      setIsImporting(false)
    }
  }

  if (!canManage) {
    return <section className="dashboard-grid settings-grid"><Card><InlineError message="Admin access required" /></Card></section>
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div>
            <h2>Import CRM data</h2>
            <p>Map and preview a CSV, then queue up to 1,000 rows. PostgreSQL schedules the upload for removal after seven days and excludes it from portable exports.</p>
          </div>
          <div className="form-grid form-grid-two">
            <Field label="Record type">
              <select className="text-input" value={entityType} onChange={(event) => selectEntity(event.target.value)} disabled={isImporting}>
                <option value="contacts">Contacts</option>
                <option value="companies">Companies</option>
              </select>
            </Field>
            <Field label="CSV file">
              <input aria-label="CSV file" className="text-input" type="file" accept=".csv,text/csv" onChange={(event) => selectFile(event.target.files?.[0])} disabled={isImporting} />
            </Field>
          </div>
          <div className="button-row">
            <Button className="button-secondary" type="button" onClick={handlePreview} disabled={!file || isPreviewing || isImporting}>
              {isPreviewing ? 'Checking…' : preview ? 'Refresh dry run' : 'Preview and map'}
            </Button>
            {preview ? <Button type="button" onClick={handleImport} disabled={isImporting || isPreviewing || mappingDirty || mappingHasErrors(preview)}>{isImporting ? 'Importing…' : 'Import valid rows'}</Button> : null}
          </div>
          {error ? <InlineError message={error} /> : null}
          {notice ? <div className="inline-note" role="status">{notice}</div> : null}

          {preview ? (
            <div className="card-stack">
              <div>
                <h3>Column mapping</h3>
                <p className="field-hint">Suggested mappings are editable. Refresh the dry run after changing a mapping.</p>
              </div>
              <div className="form-grid form-grid-two">
                {(preview.fields || []).map((field) => (
                  <Field key={field.key} label={`${field.label}${field.required ? ' (required)' : ''}`}>
                    <select
                      aria-label={`${field.label} column`}
                      className="text-input"
                      value={mapping[field.key] || ''}
                      onChange={(event) => {
                        setMapping((current) => ({ ...current, [field.key]: event.target.value }))
                        setMappingDirty(true)
                      }}
                    >
                      <option value="">Do not import</option>
                      {(preview.sourceColumns || []).map((column) => <option key={column} value={column}>{column}</option>)}
                    </select>
                  </Field>
                ))}
              </div>
              {(preview.mappingErrors || []).map((issue, index) => <InlineError key={`${issue.column}-${index}`} message={`${issue.column ? `${issue.column}: ` : ''}${issue.message}`} />)}
              <div className="inline-note" role="status">
                <strong>{preview.summary?.validRows || 0} valid</strong> · {preview.summary?.errorRows || 0} with errors · {preview.summary?.totalRows || 0} total
              </div>
              <div className="table-scroll">
                <table className="data-table">
                  <thead><tr><th>Row</th><th>Mapped data</th><th>Review</th></tr></thead>
                  <tbody>
                    {(preview.rows || []).slice(0, 5).map((row) => (
                      <tr key={row.rowNumber}>
                        <td>{row.rowNumber}</td>
                        <td>{Object.entries(row.values || {}).filter(([, value]) => value).slice(0, 4).map(([key, value]) => `${key}: ${value}`).join(' · ') || 'Empty row'}</td>
                        <td>{(row.errors || []).map((issue) => issue.message).join(' · ') || (row.warnings || []).map((issue) => issue.message).join(' · ') || 'Ready'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          ) : null}
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div><h2>Import history</h2><p>Track outcomes, download row errors, and safely reverse unchanged records.</p></div>
            <Button className="button-secondary" type="button" onClick={() => loadHistory()}>Refresh</Button>
          </div>
          <div className="record-list" role="list" aria-label="Import history">
            {batches.length === 0 ? <article className="record-row" role="listitem"><div><p>No imports yet.</p></div></article> : batches.map((batch) => (
              <article className={batch.status === 'partially_rolled_back' ? 'record-row record-row-alert' : 'record-row'} key={batch.id} role="listitem">
                <div>
                  <h3>{batch.originalFilename} · {statusLabel(batch.status)}</h3>
                  <p>{batch.processedRows} / {batch.totalRows} processed · {batch.successRows} imported · {batch.errorRows} errors</p>
                  <p className="field-hint">Started by {batch.createdByName || 'an admin'} {formatTimestamp(batch.createdAt)}</p>
                  {batch.jobStatus ? <p className="field-hint">Worker: {statusLabel(batch.jobStatus)} · attempt {batch.jobAttempts}/{batch.jobMaxAttempts}</p> : null}
                  {batch.failureMessage ? <p className="field-hint">{batch.failureMessage}</p> : null}
                  {batch.rollbackSkippedRows > 0 ? <p>{batch.rollbackSkippedRows} changed records were left active during rollback.</p> : null}
                </div>
                <div className="button-row">
                  {batch.errorRows > 0 || batch.rollbackSkippedRows > 0 ? <a className="button button-secondary" href={importErrorsURL(batch.id)}>Download errors</a> : null}
                  {batch.jobStatus === 'dead' ? <a className="button button-secondary" href="/settings/operations">Review in Operations</a> : null}
                  {['processing', 'failed'].includes(batch.status) && !batch.jobStatus ? (
                    <Button className="button-secondary" type="button" onClick={() => handleResume(batch)} disabled={isImporting}>
                      Resume with selected file
                    </Button>
                  ) : null}
                  {['completed', 'completed_with_errors', 'failed'].includes(batch.status) && batch.successRows > 0 ? (
                    <Button className="button-secondary" type="button" onClick={() => handleRollback(batch)} disabled={rollingBackId === batch.id}>
                      {rollingBackId === batch.id ? 'Rolling back…' : 'Roll back import'}
                    </Button>
                  ) : null}
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
    </section>
  )
}

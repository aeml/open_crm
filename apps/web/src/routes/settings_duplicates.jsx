import { useEffect, useMemo, useRef, useState } from 'react'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { mergeDuplicate, reviewDuplicates } from '../lib/duplicates'
import { createIdempotencyKey } from '../lib/idempotency'
import { usePageTitle } from '../lib/use_page_title'

function fieldMap(record) {
  return Object.fromEntries((record?.fields || []).map((field) => [field.key, field]))
}

function isUnset(field) {
  if (field?.key?.startsWith('custom:')) {
    return field.value === undefined || field.value === null || field.value === ''
  }
  return !field?.value || field.value === '0' || field.value === 'false'
}

function relatedSummary(record) {
  const entries = Object.entries(record?.related || {}).filter(([, count]) => Number(count) > 0)
  return entries.length === 0 ? 'No linked work found' : entries.map(([label, count]) => `${count} ${label}`).join(' · ')
}

function recordIdentifier(record) {
  const fields = fieldMap(record)
  return fields.email?.displayValue || fields.website?.displayValue || fields.phone?.displayValue || `#${record.id}`
}

function mergeSummary(operation) {
  const moved = Object.values(operation?.relationshipCounts || {}).reduce((total, count) => total + Number(count || 0), 0)
  return `${operation.sourceLabel || `Record #${operation.sourceEntityId}`} merged into ${operation.targetLabel || `record #${operation.targetEntityId}`} · ${moved} linked item${moved === 1 ? '' : 's'} preserved`
}

export function SettingsDuplicatesRoute() {
  const { session } = useAuth()
  usePageTitle('Data quality')
  const canManage = useMemo(() => ['owner', 'admin'].includes(session?.membership?.role || ''), [session])
  const [entityType, setEntityType] = useState('contact')
  const [review, setReview] = useState({ candidates: [], recentMerges: [] })
  const [selection, setSelection] = useState(null)
  const [fieldChoices, setFieldChoices] = useState({})
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [isMerging, setIsMerging] = useState(false)
  const requestRef = useRef({ signature: '', key: '' })

  async function loadReview({ signal, type = entityType } = {}) {
    if (!canManage) return
    setIsLoading(true)
    try {
      setReview(await reviewDuplicates({ entityType: type, signal }))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to review possible duplicates.')
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadReview({ signal: controller.signal })
    return () => controller.abort()
  }, [canManage, entityType])

  function changeEntityType(nextType) {
    setEntityType(nextType)
    setSelection(null)
    setFieldChoices({})
    setNotice('')
    requestRef.current = { signature: '', key: '' }
  }

  function chooseSurvivor(candidate, target) {
    const source = candidate.first.id === target.id ? candidate.second : candidate.first
    const targetFields = fieldMap(target)
    const defaults = {}
    for (const sourceField of source.fields || []) {
      const targetField = targetFields[sourceField.key]
      if (sourceField.selectable && targetField?.selectable) {
        defaults[sourceField.key] = isUnset(targetField) && !isUnset(sourceField) ? source.id : target.id
      }
    }
    setSelection({ candidate, source, target })
    setFieldChoices(defaults)
    setError('')
    setNotice('')
    requestRef.current = { signature: '', key: '' }
  }

  async function handleMerge() {
    if (!selection) return
    const { source, target } = selection
    const input = {
      entityType,
      sourceEntityId: source.id,
      targetEntityId: target.id,
      sourceFields: Object.entries(fieldChoices).filter(([, recordId]) => recordId === source.id).map(([field]) => field),
      sourceUpdatedAt: source.updatedAt,
      targetUpdatedAt: target.updatedAt
    }
    const signature = JSON.stringify(input)
    if (requestRef.current.signature !== signature) requestRef.current = { signature, key: createIdempotencyKey('merge') }
    if (!window.confirm(`Permanently merge “${source.label}” into “${target.label}”? The duplicate will be archived and linked work will move to the survivor. This merge cannot be automatically undone.`)) return
    setIsMerging(true)
    setError('')
    setNotice('')
    try {
      const operation = await mergeDuplicate({ ...input, idempotencyKey: requestRef.current.key })
      setNotice(`Merge complete. ${mergeSummary(operation)}.`)
      setSelection(null)
      setFieldChoices({})
      requestRef.current = { signature: '', key: '' }
      await loadReview()
    } catch (mergeError) {
      setError(mergeError.message || 'Unable to merge duplicate records. Reload if either record changed.')
    } finally {
      setIsMerging(false)
    }
  }

  if (!canManage) {
    return <section className="dashboard-grid settings-grid"><Card><InlineError message="Admin access required" /></Card></section>
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div><h2>Duplicate review</h2><p>Compare likely matches, choose the surviving record, and resolve each differing field before merging.</p></div>
            <Button className="button-secondary" type="button" onClick={() => loadReview()} disabled={isLoading}>{isLoading ? 'Checking…' : 'Refresh'}</Button>
          </div>
          <Field label="Record type">
            <select className="text-input" value={entityType} onChange={(event) => changeEntityType(event.target.value)} disabled={isMerging}>
              <option value="contact">Contacts</option>
              <option value="company">Clients</option>
            </select>
          </Field>
          <div className="inline-note"><strong>Permanent operation:</strong> merging archives the duplicate and consolidates its linked work. Import, bulk-operation, and audit ledgers keep their original record IDs for historical accuracy.</div>
          {error ? <InlineError message={error} onRetry={() => loadReview()} retryLabel="Reload duplicate review" /> : null}
          {notice ? <div className="inline-note" role="status">{notice}</div> : null}
          {!isLoading && review.candidates.length === 0 ? <p className="empty-state">No likely duplicate {entityType === 'contact' ? 'contacts' : 'clients'} found.</p> : null}
          <div className="record-list" role="list" aria-label="Possible duplicate pairs">
            {review.candidates.map((candidate) => (
              <article className="record-row duplicate-candidate-row" role="listitem" key={`${candidate.entityType}-${candidate.first.id}-${candidate.second.id}`}>
                <div>
                  <h3>{candidate.first.label} / {candidate.second.label}</h3>
                  <p>{candidate.reasons.join(' · ')}</p>
                  <p className="field-hint">{candidate.first.label} ({recordIdentifier(candidate.first)}): {relatedSummary(candidate.first)}</p>
                  <p className="field-hint">{candidate.second.label} ({recordIdentifier(candidate.second)}): {relatedSummary(candidate.second)}</p>
                </div>
                <div className="button-row">
                  <Button aria-label={`Keep ${candidate.first.label} (${recordIdentifier(candidate.first)})`} className="button-secondary" type="button" onClick={() => chooseSurvivor(candidate, candidate.first)}>Keep {candidate.first.label}</Button>
                  <Button aria-label={`Keep ${candidate.second.label} (${recordIdentifier(candidate.second)})`} className="button-secondary" type="button" onClick={() => chooseSurvivor(candidate, candidate.second)}>Keep {candidate.second.label}</Button>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div><h2>{selection ? `Merge into ${selection.target.label}` : 'Field resolution'}</h2><p>{selection ? `Choose which value survives. Linked work from ${selection.source.label} is consolidated automatically.` : 'Choose a survivor from a possible duplicate pair.'}</p></div>
          {selection ? (
            <>
              <div className="table-scroll">
                <table className="data-table">
                  <thead><tr><th>Field</th><th>Keep from {selection.target.label}</th><th>Use from {selection.source.label}</th></tr></thead>
                  <tbody>
                    {selection.target.fields.map((targetField) => {
                      const sourceField = fieldMap(selection.source)[targetField.key]
                      if (!sourceField || targetField.value === sourceField.value) return null
                      if (!targetField.selectable || !sourceField.selectable) return <tr key={targetField.key}><td>{targetField.label}</td><td colSpan="2">{targetField.displayValue} / {sourceField.displayValue} · safest value is retained automatically</td></tr>
                      return (
                        <tr key={targetField.key}>
                          <td>{targetField.label}</td>
                          <td><label><input type="radio" name={`merge-${targetField.key}`} checked={fieldChoices[targetField.key] === selection.target.id} onChange={() => setFieldChoices((current) => ({ ...current, [targetField.key]: selection.target.id }))} /> {targetField.displayValue}</label></td>
                          <td><label><input type="radio" name={`merge-${targetField.key}`} checked={fieldChoices[targetField.key] === selection.source.id} onChange={() => setFieldChoices((current) => ({ ...current, [targetField.key]: selection.source.id }))} /> {sourceField.displayValue}</label></td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
              <div className="button-row">
                <Button className="button-danger" type="button" onClick={handleMerge} disabled={isMerging}>{isMerging ? 'Merging…' : `Merge and archive ${selection.source.label}`}</Button>
                <Button className="button-secondary" type="button" onClick={() => setSelection(null)} disabled={isMerging}>Cancel</Button>
              </div>
            </>
          ) : null}
          <details>
            <summary>Recent permanent merges</summary>
            <div className="record-list" role="list" aria-label="Recent duplicate merges">
              {review.recentMerges.length === 0 ? <p className="field-hint">No merges recorded yet.</p> : review.recentMerges.map((operation) => <article className="record-row" role="listitem" key={operation.id}><div><p>{mergeSummary(operation)}</p><p className="field-hint">{new Date(operation.createdAt).toLocaleString()}</p></div></article>)}
            </div>
          </details>
        </div>
      </Card>
    </section>
  )
}

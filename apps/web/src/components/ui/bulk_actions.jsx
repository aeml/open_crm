import { useMemo, useRef, useState } from 'react'
import { executeBulkOperation, listBulkOperations, rollbackBulkOperation } from '../../lib/bulk_operations'
import { isAbortError } from '../../lib/api'
import { Button } from './button'
import { Field } from './field'

export const bulkStatusOptions = {
  contact: [{ value: 'lead', label: 'Lead' }, { value: 'prospect', label: 'Prospect' }, { value: 'customer', label: 'Customer' }],
  company: [{ value: 'lead', label: 'Lead' }, { value: 'prospect', label: 'Prospect' }, { value: 'customer', label: 'Customer' }],
  deal: [{ value: 'open', label: 'Open' }, { value: 'won', label: 'Won' }, { value: 'lost', label: 'Lost' }],
  task: [{ value: 'open', label: 'Open' }, { value: 'completed', label: 'Completed' }]
}

function operationLabel(operation) {
  if (operation.action === 'archive') return 'Archive'
  if (operation.action === 'reassign') return operation.targetUserName ? `Assign to ${operation.targetUserName}` : 'Set unassigned'
  return `Set status to ${operation.actionValue || 'unknown'}`
}

function operationOutcome(operation) {
  if (operation.status === 'partially_rolled_back') return `${operation.rolledBackCount} restored, ${operation.rollbackSkippedCount} kept because they changed later`
  if (operation.status === 'rolled_back') return `${operation.rolledBackCount} restored`
  return `${operation.changedCount} of ${operation.targetCount} changed`
}

function operationTime(value) {
  const timestamp = new Date(value)
  return Number.isNaN(timestamp.getTime()) ? 'time unavailable' : timestamp.toLocaleString()
}

function requestKey() {
  if (globalThis.crypto?.randomUUID) return `bulk-${globalThis.crypto.randomUUID()}`
  return `bulk-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

export function BulkActions({ entityType, selectedIds, visibleIds, onSelectionChange, onChanged, statuses, userOptions = [] }) {
  const [action, setAction] = useState('reassign')
  const [actionValue, setActionValue] = useState(statuses[0]?.value || '')
  const [targetUserId, setTargetUserId] = useState('')
  const [operations, setOperations] = useState(null)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [isApplying, setIsApplying] = useState(false)
  const [rollingBackId, setRollingBackId] = useState(0)
  const requestRef = useRef({ signature: '', key: '' })
  const activeUsers = useMemo(() => userOptions.filter((user) => user.membershipStatus !== 'disabled'), [userOptions])
  const allVisibleSelected = visibleIds.length > 0 && visibleIds.every((id) => selectedIds.includes(id))

  async function loadOperations({ signal } = {}) {
    try {
      setOperations(await listBulkOperations({ entityType, limit: 5, signal }))
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load recent bulk changes.')
    }
  }

  function toggleAllVisible() {
    onSelectionChange(allVisibleSelected ? [] : visibleIds.slice(0, 100))
  }

  async function handleApply() {
    if (selectedIds.length === 0) return
    if (action === 'archive' && !window.confirm(`Archive ${selectedIds.length} selected ${entityType} records? You can undo this from recent bulk changes.`)) return
    const request = {
      entityType,
      action,
      actionValue: action === 'set_status' ? actionValue : '',
      entityIds: selectedIds.slice(0, 100)
    }
    if (action === 'reassign') request.targetUserId = Number.parseInt(targetUserId, 10) || 0
    const signature = JSON.stringify(request)
    if (requestRef.current.signature !== signature) requestRef.current = { signature, key: requestKey() }
    setIsApplying(true)
    setError('')
    setNotice('')
    try {
      const operation = await executeBulkOperation({ ...request, idempotencyKey: requestRef.current.key })
      setNotice(`${operationLabel(operation)}: ${operationOutcome(operation)}.`)
      requestRef.current = { signature: '', key: '' }
      onSelectionChange([])
      await onChanged()
      await loadOperations()
    } catch (applyError) {
      setError(applyError.message || 'Unable to apply bulk change.')
    } finally {
      setIsApplying(false)
    }
  }

  async function handleRollback(operation) {
    if (!window.confirm(`Undo “${operationLabel(operation)}”? Records edited afterward will be left unchanged.`)) return
    setRollingBackId(operation.id)
    setError('')
    setNotice('')
    try {
      const result = await rollbackBulkOperation(operation.id)
      setNotice(`Undo complete: ${operationOutcome(result)}.`)
      await onChanged()
      await loadOperations()
    } catch (rollbackError) {
      setError(rollbackError.message || 'Unable to undo bulk change.')
    } finally {
      setRollingBackId(0)
    }
  }

  return (
    <div className="inline-note card-stack bulk-actions" aria-label={`Bulk actions for ${entityType} records`}>
      <div className="section-header">
        <div><h3>Bulk actions</h3><p>{selectedIds.length} selected · maximum 100 per operation.</p></div>
        <div className="button-row">
          <Button className="button-secondary" type="button" onClick={toggleAllVisible} disabled={visibleIds.length === 0}>{allVisibleSelected ? 'Clear selection' : 'Select current page'}</Button>
          {selectedIds.length > 0 ? <Button className="button-secondary" type="button" onClick={() => onSelectionChange([])}>Clear</Button> : null}
        </div>
      </div>
      <div className="form-grid form-grid-two">
        <Field label="Bulk change">
          <select className="text-input" value={action} onChange={(event) => setAction(event.target.value)}>
            <option value="reassign">Reassign owner</option>
            <option value="set_status">Change status</option>
            <option value="archive">Archive</option>
          </select>
        </Field>
        {action === 'reassign' ? (
          <Field label={entityType === 'task' ? 'New assignee' : 'New owner'}>
            <select className="text-input" value={targetUserId} onChange={(event) => setTargetUserId(event.target.value)}>
              <option value="">Unassigned</option>
              {activeUsers.map((user) => <option key={user.id} value={user.id}>{`${user.firstName || ''} ${user.lastName || ''}`.trim() || user.email}</option>)}
            </select>
          </Field>
        ) : action === 'set_status' ? (
          <Field label="New status">
            <select className="text-input" value={actionValue} onChange={(event) => setActionValue(event.target.value)}>
              {statuses.map((status) => <option key={status.value} value={status.value}>{status.label}</option>)}
            </select>
          </Field>
        ) : <div className="field-hint">Archived records disappear from active lists. Undo remains change-aware.</div>}
      </div>
      <div><Button type="button" onClick={handleApply} disabled={selectedIds.length === 0 || isApplying}>{isApplying ? 'Applying...' : `Apply to ${selectedIds.length} selected`}</Button></div>
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      {notice ? <p role="status">{notice}</p> : null}
      <details onToggle={(event) => { if (event.currentTarget.open && operations === null) loadOperations() }}>
          <summary>Recent bulk changes</summary>
          <div className="record-list" role="list" aria-label={`Recent ${entityType} bulk changes`}>
            {operations === null ? <p className="field-hint">Loading recent changes...</p> : operations.length === 0 ? <p className="field-hint">No bulk changes recorded yet.</p> : operations.map((operation) => (
              <article className="record-row" key={operation.id} role="listitem">
                <div><p>{operationLabel(operation)}</p><p className="field-hint">{operationOutcome(operation)} · {operationTime(operation.createdAt)}</p></div>
                {operation.status === 'completed' ? <Button className="button-secondary" type="button" onClick={() => handleRollback(operation)} disabled={rollingBackId === operation.id}>{rollingBackId === operation.id ? 'Undoing...' : 'Undo'}</Button> : null}
              </article>
            ))}
          </div>
      </details>
    </div>
  )
}

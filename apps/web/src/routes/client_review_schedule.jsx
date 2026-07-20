import { useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { deleteClientReview, getClientReview, upsertClientReview } from '../lib/client_reviews'
import { listTasks } from '../lib/tasks'

const emptySchedule = { exists: false, semantics: [] }

export async function refreshClientReviewTasks(entityType, entityId, setDetail, setDetailCache) {
  if (!entityId) return
  const taskData = await listTasks({ status: 'open', entityType, entityId })
  setDetail((current) => {
    if (!current) return current
    const next = { ...current, tasks: taskData.tasks || [] }
    setDetailCache((cache) => ({ ...cache, [entityId]: next }))
    return next
  })
}

function toDatetimeLocal(value) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
  return local.toISOString().slice(0, 16)
}

function toISOString(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toISOString()
}

function initialForm(schedule, users) {
  return {
    reviewType: schedule?.reviewType || 'review',
    nextReviewAt: toDatetimeLocal(schedule?.nextReviewAt),
    cadenceMonths: String(schedule?.cadenceMonths ?? 0),
    assignedToUserId: schedule?.assignedToUserId ? String(schedule.assignedToUserId) : (users[0]?.id ? String(users[0].id) : '')
  }
}

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not scheduled' : date.toLocaleString()
}

export function ClientReviewSchedule({ entityType, entityId, isClient, canWrite, users = [], onChanged }) {
  const activeRecordRef = useRef({ entityType, entityId })
  if (activeRecordRef.current.entityType !== entityType || activeRecordRef.current.entityId !== entityId) {
    activeRecordRef.current = { entityType, entityId }
  }
  const [schedule, setSchedule] = useState(emptySchedule)
  const [form, setForm] = useState(() => initialForm(emptySchedule, users))
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [isLoading, setIsLoading] = useState(Boolean(isClient && entityId))
  const [isSaving, setIsSaving] = useState(false)
  const [run, setRun] = useState(0)

  useEffect(() => {
    if (!isClient || !entityId) {
      setSchedule(emptySchedule)
      setIsLoading(false)
      return undefined
    }
    const record = activeRecordRef.current
    const controller = new AbortController()
    setIsLoading(true)
    getClientReview(entityType, entityId, { signal: controller.signal })
      .then((result) => {
        if (activeRecordRef.current !== record) return
        const next = result || emptySchedule
        if (next.entityType !== record.entityType || next.entityId !== record.entityId) throw new Error('Unable to load client review schedule.')
        setSchedule(next)
        setForm(initialForm(next, users))
        setError('')
      })
      .catch((loadError) => {
        if (!isAbortError(loadError) && activeRecordRef.current === record) setError(loadError.message || 'Unable to load client review schedule.')
      })
      .finally(() => {
        if (!controller.signal.aborted && activeRecordRef.current === record) setIsLoading(false)
      })
    return () => controller.abort()
  }, [entityType, entityId, isClient, run, users])

  if (!isClient) return null

  async function refreshParent(record) {
    if (!onChanged || activeRecordRef.current !== record) return
    try {
      await onChanged()
    } catch {
      // The schedule is already durable; its own state remains authoritative.
    }
  }

  async function handleSave(event) {
    event.preventDefault()
    const record = activeRecordRef.current
    const nextReviewAt = toISOString(form.nextReviewAt)
    if (!nextReviewAt || !form.assignedToUserId) {
      setError('Choose a due time and an active assignee.')
      return
    }
    setIsSaving(true)
    try {
      const next = await upsertClientReview(record.entityType, record.entityId, {
        reviewType: form.reviewType,
        nextReviewAt,
        cadenceMonths: Number.parseInt(form.cadenceMonths, 10) || 0,
        assignedToUserId: Number.parseInt(form.assignedToUserId, 10) || 0
      })
      if (activeRecordRef.current !== record) return
      if (next?.entityType !== record.entityType || next?.entityId !== record.entityId) throw new Error('Unable to save client review schedule.')
      setSchedule(next)
      setForm(initialForm(next, users))
      setNotice(next?.exists ? `${next.reviewLabel} task scheduled.` : 'Client review schedule saved.')
      setError('')
      await refreshParent(record)
    } catch (saveError) {
      if (activeRecordRef.current === record) setError(saveError.message || 'Unable to save client review schedule.')
    } finally {
      if (activeRecordRef.current === record) setIsSaving(false)
    }
  }

  async function handleClear() {
    const record = activeRecordRef.current
    setIsSaving(true)
    try {
      await deleteClientReview(record.entityType, record.entityId)
      if (activeRecordRef.current !== record) return
      setSchedule(emptySchedule)
      setForm(initialForm(emptySchedule, users))
      setNotice('Client review schedule cleared; its open generated task was archived.')
      setError('')
      await refreshParent(record)
    } catch (clearError) {
      if (activeRecordRef.current === record) setError(clearError.message || 'Unable to clear client review schedule.')
    } finally {
      if (activeRecordRef.current === record) setIsSaving(false)
    }
  }

  return (
    <Card className="client-review-card">
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Client review schedule</h3>
            <p>Create one assigned task now; recurring completion schedules the next obligation automatically.</p>
          </div>
          {schedule.exists ? <span className="chip">{schedule.isOverdue ? 'Overdue' : schedule.taskStatus === 'completed' ? 'Completed' : 'Scheduled'}</span> : null}
        </div>
        {isLoading ? <p className="field-hint" role="status">Loading client review schedule...</p> : null}
        {error ? <InlineError message={error} onRetry={() => setRun((current) => current + 1)} retryLabel="Retry client review" /> : null}
        {notice ? <p className="field-hint" role="status">{notice}</p> : null}
        {!isLoading && schedule.exists ? (
          <div className="record-list" role="list" aria-label="Current client review obligation">
            <article className="record-row" role="listitem">
              <div>
                <h4>{schedule.reviewLabel}</h4>
                <p>{formatTimestamp(schedule.nextReviewAt)} · {schedule.cadenceLabel}</p>
                <p className="field-hint">Assigned to {schedule.assignedToUserName || 'active team member'}</p>
              </div>
              <Link to={`/tasks/${schedule.currentTaskId}`}>Open task</Link>
            </article>
          </div>
        ) : null}
        {!isLoading && !schedule.exists ? <p className="field-hint">No review or renewal task is scheduled for this client.</p> : null}
        {canWrite ? (
          <form className="auth-form" onSubmit={handleSave}>
            <Field label="Follow-up type">
              <select className="text-input" value={form.reviewType} onChange={(event) => setForm((current) => ({ ...current, reviewType: event.target.value }))}>
                <option value="review">Client review</option>
                <option value="renewal">Client renewal</option>
              </select>
            </Field>
            <Field label="Next due time">
              <input className="text-input" type="datetime-local" value={form.nextReviewAt} onChange={(event) => setForm((current) => ({ ...current, nextReviewAt: event.target.value }))} required />
            </Field>
            <Field label="Cadence" hint="One time finishes on completion. Recurring schedules skip missed periods and create only the next future task.">
              <select className="text-input" value={form.cadenceMonths} onChange={(event) => setForm((current) => ({ ...current, cadenceMonths: event.target.value }))}>
                <option value="0">One time</option>
                <option value="1">Every month</option>
                <option value="3">Every 3 months</option>
                <option value="6">Every 6 months</option>
                <option value="12">Every 12 months</option>
              </select>
            </Field>
            <Field label="Assignee">
              <select className="text-input" value={form.assignedToUserId} onChange={(event) => setForm((current) => ({ ...current, assignedToUserId: event.target.value }))} required>
                <option value="">Choose a team member</option>
                {users.map((user) => <option key={user.id} value={user.id}>{`${user.firstName || ''} ${user.lastName || ''}`.trim() || user.email}</option>)}
              </select>
            </Field>
            <div className="button-row">
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving…' : schedule.exists ? 'Update schedule' : 'Schedule task'}</Button>
              {schedule.exists ? <Button className="button-danger" type="button" onClick={handleClear} disabled={isSaving}>Clear schedule</Button> : null}
            </div>
          </form>
        ) : null}
        {schedule.semantics?.length ? <details><summary>How recurring review tasks work</summary><ul>{schedule.semantics.map((rule) => <li key={rule}>{rule}</li>)}</ul></details> : null}
      </div>
    </Card>
  )
}

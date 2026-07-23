import { useEffect, useMemo, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listReportSchedules, resolveReportRecipientDelivery, upsertReportSchedule } from '../lib/reports'
import { listOrganizationUsers } from '../lib/users'
import { isExecutableReportDefinition } from './report_definition_model'

const weekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

function emptyDraft(userId = 0) {
  return { revision: 0, cadence: 'weekly', weekdayUtc: 1, hourUtc: 13, recipientUserIds: userId ? [userId] : [], isActive: true }
}

function draftFromSchedule(schedule, userId = 0) {
  if (!schedule) return emptyDraft(userId)
  return {
    revision: schedule.revision,
    cadence: schedule.cadence,
    weekdayUtc: schedule.weekdayUtc ?? 1,
    hourUtc: schedule.hourUtc,
    recipientUserIds: schedule.recipients.map((recipient) => recipient.userId),
    isActive: schedule.isActive
  }
}

function formatUTC(value) {
  if (!value) return 'Not scheduled'
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short', timeZone: 'UTC' }).format(new Date(value)) + ' UTC'
}

function deliveryLabel(status) {
  return ({ accepted: 'Accepted by provider', uncertain: 'Needs review', failed: 'Failed', skipped: 'Skipped', sending: 'Sending', pending: 'Pending' })[status] || status
}

export function ScheduledReportDelivery({ definitions = [] }) {
  const executableDefinitions = useMemo(() => definitions.filter(isExecutableReportDefinition), [definitions])
  const [overview, setOverview] = useState(null)
  const [users, setUsers] = useState([])
  const [definitionId, setDefinitionId] = useState('')
  const [draft, setDraft] = useState(emptyDraft())
  const [confirmedDuplicateRisk, setConfirmedDuplicateRisk] = useState(new Set())
  const [isLoading, setIsLoading] = useState(true)
  const [pendingAction, setPendingAction] = useState('')
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')
  const requestRef = useRef(null)
  const requestVersion = useRef(0)
  const mutationPending = useRef(false)

  async function loadDelivery({ preserveSelection = true } = {}) {
    requestRef.current?.abort()
    const controller = new AbortController()
    requestRef.current = controller
    const version = requestVersion.current + 1
    requestVersion.current = version
    setIsLoading(true)
    setError('')
    try {
      const [loadedOverview, loadedUsers] = await Promise.all([
        listReportSchedules({ signal: controller.signal }),
        listOrganizationUsers({ signal: controller.signal })
      ])
      if (controller.signal.aborted || requestVersion.current !== version) return
      setOverview(loadedOverview)
      setUsers(loadedUsers)
      const candidateId = preserveSelection && definitionId && executableDefinitions.some((definition) => String(definition.id) === String(definitionId))
        ? String(definitionId)
        : String(loadedOverview.schedules[0]?.reportDefinitionId || executableDefinitions[0]?.id || '')
      setDefinitionId(candidateId)
      const schedule = loadedOverview.schedules.find((item) => String(item.reportDefinitionId) === candidateId)
      setDraft(draftFromSchedule(schedule, loadedUsers[0]?.id))
      setConfirmedDuplicateRisk(new Set())
    } catch (loadError) {
      if (!isAbortError(loadError) && requestVersion.current === version) setError(loadError.message || 'Unable to load scheduled report delivery.')
    } finally {
      if (requestRef.current === controller) requestRef.current = null
      if (!controller.signal.aborted && requestVersion.current === version) setIsLoading(false)
    }
  }

  useEffect(() => {
    loadDelivery({ preserveSelection: false })
    return () => requestRef.current?.abort()
  }, [])

  function chooseDefinition(nextDefinitionId) {
    setDefinitionId(nextDefinitionId)
    const schedule = overview?.schedules.find((item) => String(item.reportDefinitionId) === String(nextDefinitionId))
    setDraft(draftFromSchedule(schedule, users[0]?.id))
    setStatus('')
  }

  function toggleRecipient(userId) {
    setDraft((current) => ({
      ...current,
      recipientUserIds: current.recipientUserIds.includes(userId)
        ? current.recipientUserIds.filter((id) => id !== userId)
        : current.recipientUserIds.length < 10 ? [...current.recipientUserIds, userId] : current.recipientUserIds
    }))
    setStatus('')
  }

  async function saveSchedule(nextActive = draft.isActive) {
    const numericDefinitionId = Number(definitionId)
    if (!numericDefinitionId || draft.recipientUserIds.length === 0 || mutationPending.current) return
    mutationPending.current = true
    setPendingAction(nextActive ? 'save' : 'pause')
    setError('')
    setStatus('')
    try {
      const input = { ...draft, isActive: nextActive, weekdayUtc: draft.cadence === 'weekly' ? draft.weekdayUtc : null }
      await upsertReportSchedule(numericDefinitionId, input)
      setStatus(nextActive ? 'Scheduled CSV delivery saved.' : 'Scheduled CSV delivery paused. Already-started provider effects are preserved as evidence.')
      await loadDelivery()
    } catch (saveError) {
      setError(saveError.message || 'Unable to save scheduled report delivery.')
    } finally {
      mutationPending.current = false
      setPendingAction('')
    }
  }

  async function resolveDelivery(delivery, resolution) {
    if (mutationPending.current) return
    const confirmDuplicateRisk = resolution === 'retry' && delivery.status === 'uncertain' && confirmedDuplicateRisk.has(delivery.id)
    mutationPending.current = true
    setPendingAction(`delivery-${delivery.id}-${resolution}`)
    setError('')
    setStatus('')
    try {
      await resolveReportRecipientDelivery(delivery.id, { resolution, confirmDuplicateRisk })
      setStatus(resolution === 'confirmed_sent' ? 'Delivery marked as sent.' : 'Exact retained CSV queued for another delivery attempt.')
      await loadDelivery()
    } catch (resolveError) {
      setError(resolveError.message || 'Unable to resolve scheduled report delivery.')
    } finally {
      mutationPending.current = false
      setPendingAction('')
    }
  }

  const selectedSchedule = overview?.schedules.find((item) => String(item.reportDefinitionId) === String(definitionId))
  const canSave = Boolean(definitionId) && draft.recipientUserIds.length > 0 && draft.recipientUserIds.length <= 10 && overview?.deliveryAvailable && !pendingAction

  return (
    <Card className="scheduled-report-delivery">
      <div className="card-stack">
        <div className="section-header">
          <div>
            <p className="eyebrow">Scheduled delivery</p>
            <h2>Saved-report CSV email</h2>
            <p>Owners and admins can send one exact generated CSV to up to ten active workspace members each day or week. Times are UTC; retained artifacts expire after seven days.</p>
          </div>
          <Button className="button-secondary" type="button" disabled={isLoading || Boolean(pendingAction)} onClick={() => loadDelivery()}>{isLoading ? 'Refreshing…' : 'Refresh delivery evidence'}</Button>
        </div>
        {isLoading && !overview ? <p className="field-hint" role="status">Loading scheduled report delivery…</p> : null}
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {error ? <InlineError message={error} onRetry={() => loadDelivery()} retryLabel="Reload scheduled delivery" /> : null}
        {overview && !overview.deliveryAvailable ? <div className="inline-note" role="status">System email delivery is not configured. An operator must configure Postmark before a schedule can be enabled.</div> : null}
        {overview ? <>
          <Field label="Saved report">
            <select className="text-input" value={definitionId} disabled={Boolean(pendingAction)} onChange={(event) => chooseDefinition(event.target.value)}>
              <option value="">Choose an active executable report</option>
              {executableDefinitions.map((definition) => <option key={definition.id} value={definition.id}>{definition.name}</option>)}
            </select>
          </Field>
          {executableDefinitions.length === 0 ? <p className="field-hint">Create and activate a table or grouped-bar saved report before scheduling delivery.</p> : null}
          {definitionId ? <div className="card-stack">
            <div className="record-row">
              <Field label="Cadence">
                <select className="text-input" value={draft.cadence} onChange={(event) => setDraft((current) => ({ ...current, cadence: event.target.value }))}>
                  <option value="daily">Daily</option>
                  <option value="weekly">Weekly</option>
                </select>
              </Field>
              {draft.cadence === 'weekly' ? <Field label="Weekday (UTC)">
                <select className="text-input" value={draft.weekdayUtc} onChange={(event) => setDraft((current) => ({ ...current, weekdayUtc: Number(event.target.value) }))}>
                  {weekdays.map((day, index) => <option key={day} value={index}>{day}</option>)}
                </select>
              </Field> : null}
              <Field label="Hour (UTC)">
                <select className="text-input" value={draft.hourUtc} onChange={(event) => setDraft((current) => ({ ...current, hourUtc: Number(event.target.value) }))}>
                  {Array.from({ length: 24 }, (_, hour) => <option key={hour} value={hour}>{String(hour).padStart(2, '0')}:00</option>)}
                </select>
              </Field>
            </div>
            <fieldset className="card-stack">
              <legend>Active workspace recipients ({draft.recipientUserIds.length}/10)</legend>
              {users.map((user) => <label className="field-hint" key={user.id}>
                <input type="checkbox" checked={draft.recipientUserIds.includes(user.id)} disabled={!draft.recipientUserIds.includes(user.id) && draft.recipientUserIds.length >= 10} onChange={() => toggleRecipient(user.id)} /> {`${user.firstName || ''} ${user.lastName || ''}`.trim() || user.email} · {user.email}
              </label>)}
              {users.length === 0 ? <p className="field-hint">No active workspace members are available.</p> : null}
            </fieldset>
            <p className="field-hint">{selectedSchedule?.isActive ? `Next occurrence: ${formatUTC(selectedSchedule.nextRunAt)}` : selectedSchedule ? 'This schedule is paused.' : 'This saved report is not scheduled yet.'}</p>
            <div className="button-row">
              <Button type="button" disabled={!canSave} onClick={() => saveSchedule(true)}>{pendingAction === 'save' ? 'Saving…' : 'Save and enable schedule'}</Button>
              {selectedSchedule?.isActive ? <Button className="button-secondary" type="button" disabled={Boolean(pendingAction)} onClick={() => saveSchedule(false)}>{pendingAction === 'pause' ? 'Pausing…' : 'Pause schedule'}</Button> : null}
            </div>
          </div> : null}

          <div className="card-stack">
            <div><h3>Delivery history</h3><p className="field-hint">The latest 20 occurrences show provider acceptance and cases that need an explicit decision. Ambiguous sends never retry automatically.</p></div>
            <div className="record-list" role="list" aria-label="Scheduled report delivery history">
              {overview.deliveryRuns.length === 0 ? <article className="record-row" role="listitem"><p>No scheduled deliveries yet.</p></article> : overview.deliveryRuns.map((run) => (
                <article className={['partial', 'failed'].includes(run.status) ? 'record-row record-row-alert' : 'record-row'} role="listitem" key={run.id}>
                  <div><h4>{run.reportName}</h4><p className="field-hint">{formatUTC(run.scheduledFor)} · {run.status} · {run.rowCount} rows · {run.byteSize} bytes</p></div>
                  <div className="record-list" role="list" aria-label={`${run.reportName} recipients`}>
                    {run.recipients.map((delivery) => <article className={['uncertain', 'failed'].includes(delivery.status) ? 'record-row record-row-alert' : 'record-row'} role="listitem" key={delivery.id}>
                      <div><p>{delivery.recipientName || delivery.recipientEmail}</p><p className="field-hint">{delivery.recipientEmail} · {deliveryLabel(delivery.status)} · {delivery.attemptCount} attempt{delivery.attemptCount === 1 ? '' : 's'}</p></div>
                      {delivery.status === 'failed' ? <Button className="button-secondary" type="button" disabled={Boolean(pendingAction)} onClick={() => resolveDelivery(delivery, 'retry')}>{pendingAction === `delivery-${delivery.id}-retry` ? 'Queueing…' : 'Retry exact CSV'}</Button> : null}
                      {delivery.status === 'uncertain' ? <div className="card-stack">
                        <Button type="button" disabled={Boolean(pendingAction)} onClick={() => resolveDelivery(delivery, 'confirmed_sent')}>{pendingAction === `delivery-${delivery.id}-confirmed_sent` ? 'Saving…' : 'Confirm sent'}</Button>
                        <label className="field-hint"><input type="checkbox" checked={confirmedDuplicateRisk.has(delivery.id)} onChange={(event) => setConfirmedDuplicateRisk((current) => { const next = new Set(current); event.target.checked ? next.add(delivery.id) : next.delete(delivery.id); return next })} /> I understand retrying may send a duplicate</label>
                        <Button className="button-secondary" type="button" disabled={Boolean(pendingAction) || !confirmedDuplicateRisk.has(delivery.id)} onClick={() => resolveDelivery(delivery, 'retry')}>Retry despite duplicate risk</Button>
                      </div> : null}
                    </article>)}
                  </div>
                </article>
              ))}
            </div>
          </div>
        </> : null}
      </div>
    </Card>
  )
}

import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listDealPipelines } from '../lib/deals'
import { createWorkflowAutomation, listWorkflowAutomationRuns, listWorkflowAutomations, updateWorkflowAutomation } from '../lib/workflow_automations'
import { usePageTitle } from '../lib/use_page_title'

const triggerOptions = [
  { value: 'created', label: 'Deal created' },
  { value: 'stage_changed', label: 'Deal moved to a stage' },
  { value: 'archived', label: 'Deal archived' }
]

function emptyForm() {
  return { name: '', event: 'created', stageId: '', title: '', description: '', dueDays: '1', isActive: true }
}

function eventFromAutomation(automation) {
  if (automation.triggerType === 'record_created') return 'created'
  if (automation.triggerType === 'stage_changed') return 'stage_changed'
  if (automation.triggerType === 'record_updated' && automation.triggerConfig?.event === 'archived') return 'archived'
  return ''
}

function isExecutableTaskRule(automation) {
  const action = automation.actions?.[0]
  const event = eventFromAutomation(automation)
  const title = String(action?.config?.title || '')
  const description = String(action?.config?.description || '')
  const delayMinutes = Number(action?.delayMinutes || 0)
  const stageID = Number(automation.triggerConfig?.stageId || 0)
  return automation.targetEntityType === 'deal' && Boolean(event) &&
    (automation.conditions || []).length === 0 && automation.actions?.length === 1 &&
    action?.type === 'create_task' && !action.scheduledAt && Boolean(title) && title.length <= 200 && description.length <= 2000 &&
    Number.isInteger(delayMinutes) && delayMinutes >= 0 && delayMinutes <= 525600 && delayMinutes % 1440 === 0 &&
    (event !== 'stage_changed' || !automation.triggerConfig?.stageId || (Number.isInteger(stageID) && stageID > 0))
}

function formFromAutomation(automation) {
  const action = automation.actions[0]
  return {
    name: automation.name || '',
    event: eventFromAutomation(automation),
    stageId: automation.triggerConfig?.stageId ? String(automation.triggerConfig.stageId) : '',
    title: action.config?.title || '',
    description: action.config?.description || '',
    dueDays: String((action.delayMinutes || 0) / 1440),
    isActive: automation.isActive === true
  }
}

function payloadFromForm(form) {
  const dueDays = Number(form.dueDays)
  if (!Number.isInteger(dueDays) || dueDays < 0 || dueDays > 365) {
    throw new Error('Due days must be a whole number from 0 to 365.')
  }
  const triggerType = form.event === 'created' ? 'record_created' : form.event === 'stage_changed' ? 'stage_changed' : 'record_updated'
  const triggerConfig = form.event === 'archived'
    ? { event: 'archived' }
    : form.event === 'stage_changed' && form.stageId
      ? { stageId: Number(form.stageId) }
      : {}
  const config = { title: form.title.trim() }
  if (form.description.trim()) config.description = form.description.trim()
  return {
    name: form.name.trim(),
    description: 'Creates one assigned follow-up task from a deal event.',
    triggerType,
    targetEntityType: 'deal',
    triggerConfig,
    conditionLogic: 'all',
    conditions: [],
    actions: [{ type: 'create_task', config, delayMinutes: dueDays * 1440 }],
    isActive: form.isActive,
    position: 0
  }
}

function triggerSummary(automation, stagesById) {
  const event = eventFromAutomation(automation)
  if (event === 'stage_changed') {
    const stageID = Number(automation.triggerConfig?.stageId || 0)
    return stageID ? `When moved to ${stagesById.get(stageID) || `stage #${stageID}`}` : 'After every real stage change'
  }
  return event === 'archived' ? 'When a deal is archived' : 'When a deal is created'
}

function dueSummary(action) {
  const delay = Number(action?.delayMinutes || 0)
  if (delay === 0) return 'due immediately'
  if (delay % 1440 === 0) {
    const days = delay / 1440
    return `due in ${days} ${days === 1 ? 'day' : 'days'}`
  }
  return `due in ${delay} minutes`
}

function formatRunTime(value) {
  const date = new Date(value)
  return value && !Number.isNaN(date.getTime()) ? date.toLocaleString() : 'Not recorded'
}

export function SettingsAutomationsRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Automations')
  const [automations, setAutomations] = useState([])
  const [runs, setRuns] = useState([])
  const [pipelines, setPipelines] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadAutomations({ signal } = {}) {
    setIsLoading(true)
    try {
      const [nextAutomations, nextRuns, nextPipelines] = await Promise.all([
        listWorkflowAutomations({ signal }),
        listWorkflowAutomationRuns({ limit: 25, signal }),
        listDealPipelines({ signal })
      ])
      setAutomations(nextAutomations)
      setRuns(nextRuns)
      setPipelines(nextPipelines)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load task automation rules.')
    } finally {
      if (!signal?.aborted) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadAutomations({ signal: controller.signal })
    return () => controller.abort()
  }, [])

  const executableRules = useMemo(() => automations.filter(isExecutableTaskRule), [automations])
  const executableIDs = useMemo(() => new Set(executableRules.map((automation) => automation.id)), [executableRules])
  const visibleRuns = useMemo(() => runs.filter((run) => executableIDs.has(run.automationId)), [executableIDs, runs])
  const hiddenDefinitions = automations.length - executableRules.length
  const stages = useMemo(() => pipelines.flatMap((pipeline) => (pipeline.stages || []).map((stage) => ({ ...stage, pipelineName: pipeline.name }))), [pipelines])
  const stagesById = useMemo(() => new Map(stages.map((stage) => [stage.id, `${stage.pipelineName} · ${stage.name}`])), [stages])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
  }

  function startEdit(automation) {
    setEditingId(automation.id)
    setForm(formFromAutomation(automation))
    setStatus('')
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage) return
    let payload
    try {
      payload = payloadFromForm(form)
    } catch (validationError) {
      setError(validationError.message)
      return
    }
    setIsSaving(true)
    setStatus('')
    try {
      if (editingId) {
        const updated = await updateWorkflowAutomation(editingId, payload)
        setAutomations((current) => current.map((automation) => (automation.id === editingId ? updated : automation)))
        setStatus('Task automation rule updated.')
      } else {
        const created = await createWorkflowAutomation(payload)
        setAutomations((current) => [created, ...current])
        setStatus('Task automation rule created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save task automation rule.')
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div>
            <p className="eyebrow">Deal follow-up</p>
            <h2>Task automation rules</h2>
            <p>Create one predictable task after a deal is created, actually changes stage, or is archived. The task is assigned to the active deal owner, falling back to the teammate who caused the event.</p>
          </div>
          {isLoading ? <p className="field-hint">Loading task automation rules...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadAutomations()} retryLabel="Retry task automations" /> : null}
          {hiddenDefinitions > 0 ? <p className="field-hint">{hiddenDefinitions} stored legacy workflow {hiddenDefinitions === 1 ? 'definition is' : 'definitions are'} hidden because this pilot surface only exposes executable deal task rules.</p> : null}
          <div className="record-list" role="list" aria-label="Task automation rules">
            {!isLoading && executableRules.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No executable task rules yet.</p><p className="field-hint">Add one bounded follow-up rule to remove a repeated manual step.</p></div></article>
            ) : executableRules.map((automation) => {
              const action = automation.actions[0]
              return (
                <article className={automation.isActive ? 'record-row' : 'record-row record-row-alert'} key={automation.id} role="listitem">
                  <div>
                    <h3>{automation.name}</h3>
                    <p>{triggerSummary(automation, stagesById)}</p>
                    <p className="field-hint">Create “{action.config.title}” · {dueSummary(action)}</p>
                  </div>
                  <div><span className="chip">{automation.isActive ? 'Active' : 'Inactive'}</span>{canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(automation)}>Edit</Button> : null}</div>
                </article>
              )
            })}
          </div>
          <div>
            <h3>Recent task automation runs</h3>
            <p className="field-hint">Committed runs are idempotent and show exactly how many tasks were created. Skipped means a legacy unsupported rule shape was safely ignored.</p>
          </div>
          <div className="record-list" role="list" aria-label="Task automation runs">
            {!isLoading && visibleRuns.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No task automation runs yet.</p><p className="field-hint">Runs appear after an active rule receives a matching deal event.</p></div></article>
            ) : visibleRuns.map((run) => (
              <article className={run.status === 'failed' ? 'record-row record-row-alert' : 'record-row'} key={run.id} role="listitem">
                <div><p>{run.automationName}</p><p className="field-hint">{formatRunTime(run.createdAt)} · {run.actionsCompleted ?? 0}/{run.actionsTotal ?? 0} tasks created</p><p>{run.lastError || run.triggerEventKey}</p></div>
                <span className="chip">{run.status}</span>
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div><h2>{editingId ? 'Edit task rule' : 'New task rule'}</h2><p className="field-hint">One event creates one task in the same transaction as the deal change. Replays cannot create a duplicate.</p></div>
            <Field label="Rule name"><input className="text-input" maxLength="120" required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Proposal follow-up" /></Field>
            <Field label="When"><select className="text-input" value={form.event} onChange={(event) => setForm({ ...form, event: event.target.value, stageId: '' })}>{triggerOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            {form.event === 'stage_changed' ? (
              <Field label="Destination stage" hint="Choose any stage to run after every real stage change."><select className="text-input" value={form.stageId} onChange={(event) => setForm({ ...form, stageId: event.target.value })}><option value="">Any stage</option>{stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.pipelineName} · {stage.name}</option>)}</select></Field>
            ) : null}
            <Field label="Task title"><input className="text-input" maxLength="200" required value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Prepare proposal" /></Field>
            <Field label="Task description"><textarea className="text-input" rows={3} maxLength="2000" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /></Field>
            <Field label="Due in days" hint="0 means immediately; maximum 365."><input className="text-input" type="number" min="0" max="365" step="1" required value={form.dueDays} onChange={(event) => setForm({ ...form, dueDays: event.target.value })} /></Field>
            <label className="field-hint"><input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active rule</label>
            <div className="button-row"><Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save task rule' : 'Create task rule'}</Button>{editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}</div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

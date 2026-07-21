import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listDealPipelines } from '../lib/deals'
import { listLeadCaptureForms } from '../lib/lead_forms'
import { listOrganizationUsers } from '../lib/users'
import { createWorkflowAutomation, listWorkflowAutomationRuns, listWorkflowAutomations, updateWorkflowAutomation } from '../lib/workflow_automations'
import { usePageTitle } from '../lib/use_page_title'

const triggerOptions = [
  { value: 'created', label: 'Deal created' },
  { value: 'stage_changed', label: 'Deal moved to a stage' },
  { value: 'archived', label: 'Deal archived' },
  { value: 'lead_form_submitted', label: 'Lead form submitted' }
]

function emptyForm() {
  return { name: '', event: 'created', stageId: '', formId: '', conditionField: '', conditionOperator: 'equals', conditionValue: '', assignedToUserId: '', title: '', description: '', dueDays: '1', isActive: true }
}

function eventFromAutomation(automation) {
  if (automation.triggerType === 'record_created') return 'created'
  if (automation.triggerType === 'stage_changed') return 'stage_changed'
  if (automation.triggerType === 'record_updated' && automation.triggerConfig?.event === 'archived') return 'archived'
  if (automation.triggerType === 'form_submitted' && automation.targetEntityType === 'lead_form') return 'lead_form_submitted'
  return ''
}

function isExecutableTaskRule(automation) {
  const action = automation.actions?.[0]
  const event = eventFromAutomation(automation)
  const title = String(action?.config?.title || '')
  const description = String(action?.config?.description || '')
  const delayMinutes = Number(action?.delayMinutes || 0)
  const stageID = Number(automation.triggerConfig?.stageId || 0)
  const sharedShape = automation.actions?.length === 1 &&
    action?.type === 'create_task' && !action.scheduledAt && Boolean(title) && title.length <= 200 && description.length <= 2000 &&
    Number.isInteger(delayMinutes) && delayMinutes >= 0 && delayMinutes <= 525600 && delayMinutes % 1440 === 0
  if (!sharedShape) return false
  if (automation.targetEntityType === 'deal') {
    return Boolean(event) && event !== 'lead_form_submitted' && (automation.conditions || []).length === 0 &&
      (event !== 'stage_changed' || !automation.triggerConfig?.stageId || (Number.isInteger(stageID) && stageID > 0))
  }
  const formID = Number(automation.triggerConfig?.formId || 0)
  const assigneeID = Number(action?.config?.assignedToUserId || 0)
  const allowedFields = new Set(['sourceUrl', 'leadSource', 'utmSource', 'utmMedium', 'utmCampaign'])
  const allowedOperators = new Set(['equals', 'notEquals', 'contains', 'exists'])
  const conditions = automation.conditions || []
  return event === 'lead_form_submitted' && conditions.length <= 1 &&
    (!automation.triggerConfig?.formId || (Number.isInteger(formID) && formID > 0)) &&
    Number.isInteger(assigneeID) && assigneeID > 0 &&
    conditions.every((condition) => allowedFields.has(condition.field) && allowedOperators.has(condition.operator) && (condition.operator === 'exists' || Boolean(String(condition.value || '').trim())))
}

function formFromAutomation(automation) {
  const action = automation.actions[0]
  return {
    name: automation.name || '',
    event: eventFromAutomation(automation),
    stageId: automation.triggerConfig?.stageId ? String(automation.triggerConfig.stageId) : '',
    formId: automation.triggerConfig?.formId ? String(automation.triggerConfig.formId) : '',
    conditionField: automation.conditions?.[0]?.field || '',
    conditionOperator: automation.conditions?.[0]?.operator || 'equals',
    conditionValue: automation.conditions?.[0]?.value || '',
    assignedToUserId: action.config?.assignedToUserId ? String(action.config.assignedToUserId) : '',
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
  const leadFollowUp = form.event === 'lead_form_submitted'
  const triggerType = form.event === 'created' ? 'record_created' : form.event === 'stage_changed' ? 'stage_changed' : leadFollowUp ? 'form_submitted' : 'record_updated'
  const triggerConfig = form.event === 'archived'
    ? { event: 'archived' }
    : form.event === 'stage_changed' && form.stageId
      ? { stageId: Number(form.stageId) }
      : leadFollowUp && form.formId
        ? { formId: Number(form.formId) }
      : {}
  const config = { title: form.title.trim() }
  if (form.description.trim()) config.description = form.description.trim()
  if (leadFollowUp) {
    const assignedToUserId = Number(form.assignedToUserId)
    if (!Number.isInteger(assignedToUserId) || assignedToUserId <= 0) throw new Error('Choose an active teammate for lead follow-up tasks.')
    config.assignedToUserId = assignedToUserId
  }
  const conditions = []
  if (leadFollowUp && form.conditionField) {
    if (form.conditionOperator !== 'exists' && !form.conditionValue.trim()) throw new Error('Enter a condition value or remove the attribution condition.')
    conditions.push({ field: form.conditionField, operator: form.conditionOperator, value: form.conditionOperator === 'exists' ? '' : form.conditionValue.trim() })
  }
  return {
    name: form.name.trim(),
    description: leadFollowUp ? 'Creates one durable assigned follow-up task from an accepted lead form submission.' : 'Creates one assigned follow-up task from a deal event.',
    triggerType,
    targetEntityType: leadFollowUp ? 'lead_form' : 'deal',
    triggerConfig,
    conditionLogic: 'all',
    conditions,
    actions: [{ type: 'create_task', config, delayMinutes: dueDays * 1440 }],
    isActive: form.isActive,
    position: 0
  }
}

function triggerSummary(automation, stagesById, formsById) {
  const event = eventFromAutomation(automation)
  if (event === 'stage_changed') {
    const stageID = Number(automation.triggerConfig?.stageId || 0)
    return stageID ? `When moved to ${stagesById.get(stageID) || `stage #${stageID}`}` : 'After every real stage change'
  }
  if (event === 'lead_form_submitted') {
    const formID = Number(automation.triggerConfig?.formId || 0)
    return formID ? `When ${formsById.get(formID) || `lead form #${formID}`} is submitted` : 'When any active lead form is submitted'
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
  const [leadForms, setLeadForms] = useState([])
  const [users, setUsers] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadAutomations({ signal } = {}) {
    setIsLoading(true)
    try {
      const [nextAutomations, nextRuns, nextPipelines, nextLeadForms, nextUsers] = await Promise.all([
        listWorkflowAutomations({ signal }),
        listWorkflowAutomationRuns({ limit: 25, signal }),
        listDealPipelines({ signal }),
        listLeadCaptureForms({ signal }),
        listOrganizationUsers({ signal })
      ])
      setAutomations(nextAutomations)
      setRuns(nextRuns)
      setPipelines(nextPipelines)
      setLeadForms(nextLeadForms)
      setUsers(nextUsers)
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

  useEffect(() => {
    if (!runs.some((run) => run.status === 'queued' || run.status === 'running')) return undefined
    const controller = new AbortController()
    let inFlight = false
    const timer = window.setInterval(async () => {
      if (inFlight) return
      inFlight = true
      try {
        setRuns(await listWorkflowAutomationRuns({ limit: 25, signal: controller.signal }))
      } catch (pollError) {
        if (!isAbortError(pollError)) setError(pollError.message || 'Unable to refresh task automation runs.')
      } finally {
        inFlight = false
      }
    }, 1000)
    return () => {
      window.clearInterval(timer)
      controller.abort()
    }
  }, [runs])

  const executableRules = useMemo(() => automations.filter(isExecutableTaskRule), [automations])
  const executableIDs = useMemo(() => new Set(executableRules.map((automation) => automation.id)), [executableRules])
  const visibleRuns = useMemo(() => runs.filter((run) => executableIDs.has(run.automationId)), [executableIDs, runs])
  const hiddenDefinitions = automations.length - executableRules.length
  const stages = useMemo(() => pipelines.flatMap((pipeline) => (pipeline.stages || []).map((stage) => ({ ...stage, pipelineName: pipeline.name }))), [pipelines])
  const stagesById = useMemo(() => new Map(stages.map((stage) => [stage.id, `${stage.pipelineName} · ${stage.name}`])), [stages])
  const formsById = useMemo(() => new Map(leadForms.map((leadForm) => [leadForm.id, leadForm.name])), [leadForms])
  const usersById = useMemo(() => new Map(users.map((user) => [user.id, `${user.firstName || ''} ${user.lastName || ''}`.trim() || user.email])), [users])

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
            <p className="eyebrow">Follow-up operations</p>
            <h2>Task automation rules</h2>
            <p>Create one predictable task after a deal event or an accepted public lead form. Deal tasks commit with the deal change. Lead tasks use the durable worker, retained submission evidence, and an exact rule snapshot.</p>
          </div>
          {isLoading ? <p className="field-hint">Loading task automation rules...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadAutomations()} retryLabel="Retry task automations" /> : null}
          {hiddenDefinitions > 0 ? <p className="field-hint">{hiddenDefinitions} stored legacy workflow {hiddenDefinitions === 1 ? 'definition is' : 'definitions are'} hidden because this pilot surface only exposes executable task rules.</p> : null}
          <div className="record-list" role="list" aria-label="Task automation rules">
            {!isLoading && executableRules.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No executable task rules yet.</p><p className="field-hint">Add one bounded follow-up rule to remove a repeated manual step.</p></div></article>
            ) : executableRules.map((automation) => {
              const action = automation.actions[0]
              return (
                <article className={automation.isActive ? 'record-row' : 'record-row record-row-alert'} key={automation.id} role="listitem">
                  <div>
                    <h3>{automation.name}</h3>
                    <p>{triggerSummary(automation, stagesById, formsById)}</p>
                    <p className="field-hint">Create “{action.config.title}” · {dueSummary(action)}{eventFromAutomation(automation) === 'lead_form_submitted' ? ` · assign to ${usersById.get(Number(action.config.assignedToUserId)) || 'unavailable teammate'}` : ''}</p>
                  </div>
                  <div><span className="chip">{automation.isActive ? 'Active' : 'Inactive'}</span>{canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(automation)}>Edit</Button> : null}</div>
                </article>
              )
            })}
          </div>
          <div>
            <h3>Recent task automation runs</h3>
            <p className="field-hint">Committed runs are idempotent and show exactly how many tasks were created. Lead runs can also be queued, cancelled, skipped when conditions no longer match, or fail safely when their assignee is unavailable.</p>
          </div>
          <div className="record-list" role="list" aria-label="Task automation runs">
            {!isLoading && visibleRuns.length === 0 ? (
              <article className="record-row" role="listitem"><div><p>No task automation runs yet.</p><p className="field-hint">Runs appear after an active rule receives a matching deal event or accepted lead submission.</p></div></article>
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
            <div><h2>{editingId ? 'Edit task rule' : 'New task rule'}</h2><p className="field-hint">Each matching event creates at most one task. Replays cannot create a duplicate.</p></div>
            <Field label="Rule name"><input className="text-input" maxLength="120" required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Proposal follow-up" /></Field>
            <Field label="When"><select className="text-input" value={form.event} onChange={(event) => setForm({ ...form, event: event.target.value, stageId: '' })}>{triggerOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            {form.event === 'stage_changed' ? (
              <Field label="Destination stage" hint="Choose any stage to run after every real stage change."><select className="text-input" value={form.stageId} onChange={(event) => setForm({ ...form, stageId: event.target.value })}><option value="">Any stage</option>{stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.pipelineName} · {stage.name}</option>)}</select></Field>
            ) : null}
            {form.event === 'lead_form_submitted' ? (
              <>
                <Field label="Lead form" hint="Choose any active form or one exact form."><select className="text-input" value={form.formId} onChange={(event) => setForm({ ...form, formId: event.target.value })}><option value="">Any active lead form</option>{leadForms.filter((leadForm) => leadForm.isActive).map((leadForm) => <option key={leadForm.id} value={leadForm.id}>{leadForm.name}</option>)}</select></Field>
                <Field label="Optional attribution condition"><select className="text-input" value={form.conditionField} onChange={(event) => setForm({ ...form, conditionField: event.target.value, conditionValue: '' })}><option value="">No condition</option><option value="leadSource">Lead source</option><option value="utmSource">UTM source</option><option value="utmMedium">UTM medium</option><option value="utmCampaign">UTM campaign</option><option value="sourceUrl">Source URL</option></select></Field>
                {form.conditionField ? <Field label="Condition"><div className="filter-row"><select className="text-input" aria-label="Condition operator" value={form.conditionOperator} onChange={(event) => setForm({ ...form, conditionOperator: event.target.value })}><option value="equals">Equals</option><option value="notEquals">Does not equal</option><option value="contains">Contains</option><option value="exists">Exists</option></select>{form.conditionOperator !== 'exists' ? <input className="text-input" aria-label="Condition value" maxLength="500" required value={form.conditionValue} onChange={(event) => setForm({ ...form, conditionValue: event.target.value })} /> : null}</div></Field> : null}
                <Field label="Assign task to"><select className="text-input" required value={form.assignedToUserId} onChange={(event) => setForm({ ...form, assignedToUserId: event.target.value })}><option value="">Choose a teammate</option>{users.map((user) => <option key={user.id} value={user.id}>{usersById.get(user.id)}</option>)}</select></Field>
              </>
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

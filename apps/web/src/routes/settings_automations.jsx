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
import { activeRunRefreshDelay } from '../lib/workflow_automation_polling'
import { usePageTitle } from '../lib/use_page_title'
import {
  conditionOperatorLabels,
  conditionOperators,
  conditionSummary,
  dealConditionDefinitions,
  emptyForm,
  equalsOperator,
  eventFromAutomation,
  existsOperator,
  formatRunTime,
  formFromAutomation,
  isExecutableTaskRule,
  leadFormEvent,
  maxDealPlanTasks,
  payloadFromForm,
  stageChangedEvent,
  taskTimingSummary,
  triggerOptions,
  triggerSummary
} from './settings_automation_task_model'

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
    const refreshDelay = activeRunRefreshDelay(runs)
    if (refreshDelay === null) return undefined
    const controller = new AbortController()
    const timer = window.setTimeout(async () => {
      try {
        setRuns(await listWorkflowAutomationRuns({ limit: 25, signal: controller.signal }))
      } catch (pollError) {
        if (!isAbortError(pollError)) setError(pollError.message || 'Unable to refresh task automation runs.')
      }
    }, refreshDelay)
    return () => {
      window.clearTimeout(timer)
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

  function addDealTask() {
    setForm((current) => ({
      ...current,
      additionalTasks: [...current.additionalTasks, { title: '', description: '', dueDays: '1' }]
    }))
  }

  function updateDealTask(index, field, value) {
    setForm((current) => ({
      ...current,
      additionalTasks: current.additionalTasks.map((task, taskIndex) => taskIndex === index ? { ...task, [field]: value } : task)
    }))
  }

  function removeDealTask(index) {
    setForm((current) => ({ ...current, additionalTasks: current.additionalTasks.filter((_, taskIndex) => taskIndex !== index) }))
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

  const changeConditionValue = (event) => setForm({ ...form, conditionValue: event.target.value })
  const dealConditionValueProps = { className: 'text-input', 'aria-label': 'Deal condition value', required: true, value: form.conditionValue, onChange: changeConditionValue }
  const dealConditionChoices = form.conditionField === 'ownerUserId'
    ? users.filter((user) => user.status === 'active').map((user) => [user.id, usersById.get(user.id)])
    : form.conditionField === 'status' ? [['open', 'Open'], ['won', 'Won'], ['lost', 'Lost']] : null

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div>
            <p className="eyebrow">Follow-up operations</p>
            <h2>Task automation rules</h2>
            <p>Create replay-safe tasks after deal events or lead forms; lead work runs durably from retained evidence.</p>
          </div>
          {isLoading ? <p className="field-hint">Loading task automation rules...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadAutomations()} retryLabel="Retry task automations" /> : null}
          {hiddenDefinitions > 0 ? <p className="field-hint">{hiddenDefinitions} unsupported stored {hiddenDefinitions === 1 ? 'definition' : 'definitions'} hidden.</p> : null}
          <div className="record-list" role="list" aria-label="Task automation rules">
            {!isLoading && executableRules.length === 0 ? (
              <article className="record-row" role="listitem"><p>No executable task rules yet.</p></article>
            ) : executableRules.map((automation) => {
              const leadFollowUp = eventFromAutomation(automation) === leadFormEvent
              return (
                <article className={automation.isActive ? 'record-row' : 'record-row record-row-alert'} key={automation.id} role="listitem">
                  <div>
                    <h3>{automation.name}</h3>
                    <p>{triggerSummary(automation, stagesById, formsById)}</p>
                    {automation.targetEntityType === 'deal' && automation.conditions?.length ? <p className="field-hint">{conditionSummary(automation, usersById)}</p> : null}
                    <p className="field-hint">{leadFollowUp ? 'One durable task from retained submission evidence.' : `${automation.actions.length}-task playbook · all tasks commit with the deal event.`}</p>
                    <ol className="field-hint" aria-label={`${automation.name} task plan`}>
                      {automation.actions.map((action, index) => <li key={`${index}-${action.config.title}`}>Create “{action.config.title}” · {taskTimingSummary(action, leadFollowUp)}{leadFollowUp ? ` · assign to ${usersById.get(Number(action.config.assignedToUserId)) || 'unavailable teammate'}` : ''}</li>)}
                    </ol>
                  </div>
                  <div><span className="chip">{automation.isActive ? 'Active' : 'Inactive'}</span>{canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(automation)}>Edit</Button> : null}</div>
                </article>
              )
            })}
          </div>
          <div>
            <h3>Recent task automation runs</h3>
            <p className="field-hint">Scheduled and terminal lead runs retain replay-safe status and task counts.</p>
          </div>
          <div className="record-list" role="list" aria-label="Task automation runs">
            {!isLoading && visibleRuns.length === 0 ? (
              <article className="record-row" role="listitem"><p>No task automation runs yet.</p></article>
            ) : visibleRuns.map((run) => (
              <article className={run.status === 'failed' ? 'record-row record-row-alert' : 'record-row'} key={run.id} role="listitem">
                <div><p>{run.automationName}</p><p className="field-hint">{formatRunTime(run.createdAt)} · {run.actionsCompleted ?? 0}/{run.actionsTotal ?? 0} tasks created</p>{run.status === 'queued' && run.scheduledAt ? <p className="field-hint">Scheduled for {formatRunTime(run.scheduledAt)}</p> : null}<p>{run.lastError || run.triggerEventKey}</p></div>
                <span className="chip">{run.status}</span>
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div><h2>{editingId ? 'Edit task rule' : 'New task rule'}</h2><p className="field-hint">A deal event can create an ordered 1–5 task playbook atomically. Lead forms create one durable task.</p></div>
            <Field label="Rule name"><input className="text-input" maxLength="120" required value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Proposal follow-up" /></Field>
            <Field label="When"><select className="text-input" value={form.event} onChange={(event) => {
              const nextEvent = event.target.value
              setForm((current) => ({ ...current, event: nextEvent, stageId: '', conditionField: '', conditionOperator: equalsOperator, conditionValue: '', additionalTasks: nextEvent === leadFormEvent ? [] : current.additionalTasks }))
            }}>{triggerOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}</select></Field>
            {form.event === stageChangedEvent ? (
              <Field label="Destination stage" hint="Choose any stage to run after every real stage change."><select className="text-input" value={form.stageId} onChange={(event) => setForm({ ...form, stageId: event.target.value })}><option value="">Any stage</option>{stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.pipelineName} · {stage.name}</option>)}</select></Field>
            ) : null}
            {form.event !== leadFormEvent ? (
              <>
                <Field label="Optional deal condition"><select className="text-input" value={form.conditionField} onChange={(event) => {
                  const field = event.target.value
                  setForm({ ...form, conditionField: field, conditionOperator: conditionOperators(field)[0] || equalsOperator, conditionValue: '' })
                }}><option value="">No condition</option>{Object.entries(dealConditionDefinitions).map(([value, [label]]) => <option key={value} value={value}>{label}</option>)}</select></Field>
                {form.conditionField ? (
                  <Field label="Deal condition">
                    <div className="filter-row">
                      <select className="text-input" aria-label="Deal condition operator" value={form.conditionOperator} onChange={(event) => setForm({ ...form, conditionOperator: event.target.value, conditionValue: event.target.value === existsOperator ? '' : form.conditionValue })}>{conditionOperators(form.conditionField).map((value) => <option key={value} value={value}>{conditionOperatorLabels[value]}</option>)}</select>
                      {form.conditionOperator !== existsOperator ? dealConditionChoices ? (
                        <select {...dealConditionValueProps}><option value="">Choose a value</option>{dealConditionChoices.map(([value, label]) => <option key={value} value={value}>{label}</option>)}</select>
                      ) : (
                        <input {...dealConditionValueProps} type={form.conditionField === 'valueAmount' ? 'number' : 'text'} min={form.conditionField === 'valueAmount' ? '0' : undefined} step={form.conditionField === 'valueAmount' ? '0.01' : undefined} maxLength={form.conditionField === 'valueCurrency' ? 3 : undefined} />
                      ) : null}
                    </div>
                  </Field>
                ) : null}
              </>
            ) : null}
            {form.event === leadFormEvent ? (
              <>
                <Field label="Lead form" hint="Choose one active form or all forms."><select className="text-input" value={form.formId} onChange={(event) => setForm({ ...form, formId: event.target.value })}><option value="">Any active lead form</option>{leadForms.filter((leadForm) => leadForm.isActive).map((leadForm) => <option key={leadForm.id} value={leadForm.id}>{leadForm.name}</option>)}</select></Field>
                <Field label="Optional attribution condition"><select className="text-input" value={form.conditionField} onChange={(event) => setForm({ ...form, conditionField: event.target.value, conditionValue: '' })}><option value="">No condition</option><option value="leadSource">Lead source</option><option value="utmSource">UTM source</option><option value="utmMedium">UTM medium</option><option value="utmCampaign">UTM campaign</option><option value="sourceUrl">Source URL</option></select></Field>
                {form.conditionField ? <Field label="Condition"><div className="filter-row"><select className="text-input" aria-label="Condition operator" value={form.conditionOperator} onChange={(event) => setForm({ ...form, conditionOperator: event.target.value })}><option value={equalsOperator}>Equals</option><option value="notEquals">Does not equal</option><option value="contains">Contains</option><option value={existsOperator}>Exists</option></select>{form.conditionOperator !== existsOperator ? <input className="text-input" aria-label="Condition value" maxLength="500" required value={form.conditionValue} onChange={changeConditionValue} /> : null}</div></Field> : null}
                <Field label="Assign task to"><select className="text-input" required value={form.assignedToUserId} onChange={(event) => setForm({ ...form, assignedToUserId: event.target.value })}><option value="">Choose a teammate</option>{users.map((user) => <option key={user.id} value={user.id}>{usersById.get(user.id)}</option>)}</select></Field>
                <Field label="Create task after days" hint="0 runs immediately; maximum 365."><input className="text-input" type="number" min="0" max="365" step="1" required value={form.waitDays} onChange={(event) => setForm({ ...form, waitDays: event.target.value })} /></Field>
              </>
            ) : null}
            <Field label="Task title"><input className="text-input" maxLength="200" required value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Prepare proposal" /></Field>
            <Field label="Task description"><textarea className="text-input" rows={3} maxLength="2000" value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /></Field>
            <Field label="Due in days" hint={form.event === leadFormEvent ? 'From task creation; 0 is immediate; maximum 365.' : '0 is immediate; maximum 365.'}><input className="text-input" type="number" min="0" max="365" step="1" required value={form.dueDays} onChange={(event) => setForm({ ...form, dueDays: event.target.value })} /></Field>
            {form.event !== leadFormEvent ? form.additionalTasks.map((task, index) => (
              <fieldset className="card-stack" key={index}>
                <legend>Task {index + 2}</legend>
                <Field label={`Task ${index + 2} title`}><input className="text-input" maxLength="200" required value={task.title} onChange={(event) => updateDealTask(index, 'title', event.target.value)} /></Field>
                <Field label={`Task ${index + 2} description`}><textarea className="text-input" rows={3} maxLength="2000" value={task.description} onChange={(event) => updateDealTask(index, 'description', event.target.value)} /></Field>
                <Field label={`Task ${index + 2} due in days`} hint="0 is immediate; maximum 365."><input className="text-input" type="number" min="0" max="365" step="1" required value={task.dueDays} onChange={(event) => updateDealTask(index, 'dueDays', event.target.value)} /></Field>
                <Button className="button-secondary" type="button" onClick={() => removeDealTask(index)}>Remove task {index + 2}</Button>
              </fieldset>
            )) : null}
            {form.event !== leadFormEvent && form.additionalTasks.length < maxDealPlanTasks - 1 ? <Button className="button-secondary" type="button" onClick={addDealTask}>Add another task</Button> : null}
            <label className="field-hint"><input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active rule</label>
            <div className="button-row"><Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save task rule' : 'Create task rule'}</Button>{editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}</div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

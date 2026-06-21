import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createWorkflowAutomation, listWorkflowAutomations, updateWorkflowAutomation } from '../lib/workflow_automations'
import { usePageTitle } from '../lib/use_page_title'

const triggerOptions = [
  { value: 'record_created', label: 'Record created' },
  { value: 'record_updated', label: 'Record updated' },
  { value: 'stage_changed', label: 'Deal stage changed' },
  { value: 'date_reached', label: 'Date reached' },
  { value: 'form_submitted', label: 'Lead form submitted' },
  { value: 'inbound_email', label: 'Inbound email received' },
  { value: 'webhook', label: 'Webhook received' }
]

const recordTargets = [
  { value: 'contact', label: 'Contact' },
  { value: 'company', label: 'Company' },
  { value: 'deal', label: 'Deal' },
  { value: 'task', label: 'Task' }
]

const conditionFieldOptionsByTarget = {
  contact: ['firstName', 'lastName', 'email', 'phone', 'status', 'ownerUserId', 'leadSource', 'utmSource', 'utmMedium', 'utmCampaign', 'jobTitle', 'city', 'state', 'country', 'leadScore', 'leadGrade'],
  company: ['name', 'clientType', 'industry', 'phone', 'website', 'status', 'city', 'state', 'country'],
  deal: ['name', 'stageId', 'stageName', 'status', 'valueAmount', 'valueCurrency', 'ownerUserId', 'companyId', 'primaryContactId', 'expectedCloseDate'],
  task: ['title', 'status', 'entityType', 'entityId', 'assignedToUserId', 'dueAt'],
  lead_form: ['formId', 'formPublicId', 'sourceUrl', 'leadSource', 'utmSource', 'utmMedium', 'utmCampaign'],
  email_message: ['fromEmail', 'toEmail', 'subject', 'direction', 'status'],
  webhook: ['event', 'source', 'payloadType']
}

const conditionOperatorOptions = [
  { value: 'equals', label: 'equals' },
  { value: 'notEquals', label: 'does not equal' },
  { value: 'contains', label: 'contains' },
  { value: 'exists', label: 'exists' },
  { value: 'greaterThan', label: 'greater than' },
  { value: 'lessThan', label: 'less than' }
]

const actionOptions = [
  { value: 'update_field', label: 'Update field' },
  { value: 'create_task', label: 'Create task' },
  { value: 'send_email', label: 'Send email' },
  { value: 'send_sms', label: 'Send SMS' },
  { value: 'assign_owner', label: 'Assign owner' },
  { value: 'add_to_sequence', label: 'Add to sequence' },
  { value: 'call_webhook', label: 'Call webhook' },
  { value: 'notify', label: 'Notify' }
]

const actionConfigFields = {
  update_field: [{ key: 'field', label: 'Field' }, { key: 'value', label: 'Value' }],
  create_task: [{ key: 'title', label: 'Task title' }],
  send_email: [{ key: 'subject', label: 'Email subject' }, { key: 'body', label: 'Email body' }],
  send_sms: [{ key: 'body', label: 'SMS body' }],
  assign_owner: [{ key: 'userId', label: 'Owner user ID' }],
  add_to_sequence: [{ key: 'sequenceId', label: 'Sequence ID' }],
  call_webhook: [{ key: 'url', label: 'Webhook URL' }],
  notify: [{ key: 'message', label: 'Notification message' }]
}

function targetOptionsForTrigger(triggerType) {
  if (triggerType === 'stage_changed') return [{ value: 'deal', label: 'Deal' }]
  if (triggerType === 'form_submitted') return [{ value: 'lead_form', label: 'Lead form' }]
  if (triggerType === 'inbound_email') return [{ value: 'email_message', label: 'Email message' }]
  if (triggerType === 'webhook') return [{ value: 'webhook', label: 'Webhook' }]
  return recordTargets
}

function conditionOptionsForTarget(target) {
  return (conditionFieldOptionsByTarget[target] || conditionFieldOptionsByTarget.contact).map((value) => ({ value, label: value }))
}

function emptyConditionDraft(target = 'contact') {
  const firstField = conditionOptionsForTarget(target)[0]?.value || ''
  return { field: firstField, operator: 'equals', value: '' }
}

function emptyActionDraft(type = 'create_task') {
  return { type, config: {} }
}

function emptyForm() {
  return {
    name: '',
    description: '',
    triggerType: 'record_created',
    targetEntityType: 'contact',
    triggerConfigText: '{}',
    conditionLogic: 'all',
    conditionsText: '[]',
    actionsText: '[]',
    isActive: false,
    position: '0'
  }
}

function formFromAutomation(automation) {
  return {
    name: automation.name || '',
    description: automation.description || '',
    triggerType: automation.triggerType || 'record_created',
    targetEntityType: automation.targetEntityType || targetOptionsForTrigger(automation.triggerType || 'record_created')[0].value,
    triggerConfigText: JSON.stringify(automation.triggerConfig || {}, null, 2),
    conditionLogic: automation.conditionLogic || 'all',
    conditionsText: JSON.stringify(automation.conditions || [], null, 2),
    actionsText: JSON.stringify(automation.actions || [], null, 2),
    isActive: automation.isActive === true,
    position: String(automation.position ?? 0)
  }
}

function parseTriggerConfig(raw) {
  const parsed = JSON.parse(raw || '{}')
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') {
    throw new Error('Trigger config must be a JSON object.')
  }
  return Object.fromEntries(Object.entries(parsed).filter(([key, value]) => key.trim() && value !== null).map(([key, value]) => [key.trim(), value]))
}

function parseConditions(raw) {
  const parsed = JSON.parse(raw || '[]')
  if (!Array.isArray(parsed)) {
    throw new Error('Conditions must be a JSON array.')
  }
  return parsed.map((condition) => ({
    field: String(condition?.field || '').trim(),
    operator: String(condition?.operator || 'equals').trim(),
    value: String(condition?.value || '').trim()
  }))
}

function parseActions(raw) {
  const parsed = JSON.parse(raw || '[]')
  if (!Array.isArray(parsed)) {
    throw new Error('Actions must be a JSON array.')
  }
  return parsed.map((action) => {
    const config = action?.config || {}
    if (!config || Array.isArray(config) || typeof config !== 'object') {
      throw new Error('Action configs must be JSON objects.')
    }
    return {
      type: String(action?.type || '').trim(),
      config: Object.fromEntries(Object.entries(config).filter(([key, value]) => key.trim() && value !== null).map(([key, value]) => [key.trim(), typeof value === 'string' ? value.trim() : value]))
    }
  })
}

function parseConditionsForBuilder(raw) {
  try {
    return parseConditions(raw)
  } catch {
    return []
  }
}

function parseActionsForBuilder(raw) {
  try {
    return parseActions(raw)
  } catch {
    return []
  }
}

function payloadFromForm(form) {
  return {
    name: form.name,
    description: form.description,
    triggerType: form.triggerType,
    targetEntityType: form.targetEntityType,
    triggerConfig: parseTriggerConfig(form.triggerConfigText),
    conditionLogic: form.conditionLogic,
    conditions: parseConditions(form.conditionsText),
    actions: parseActions(form.actionsText),
    isActive: form.isActive,
    position: Number.parseInt(String(form.position || 0), 10) || 0
  }
}

function triggerLabel(value) {
  return triggerOptions.find((option) => option.value === value)?.label || value
}

function targetLabel(triggerType, value) {
  return targetOptionsForTrigger(triggerType).find((option) => option.value === value)?.label || value
}

function actionLabel(value) {
  return actionOptions.find((option) => option.value === value)?.label || value
}

function conditionSummary(condition) {
  if (condition.operator === 'exists') {
    return `${condition.field} exists`
  }
  return `${condition.field} ${condition.operator || 'equals'} ${condition.value}`
}

function actionSummary(action) {
  const config = action.config || {}
  const primary = config.title || config.subject || config.message || config.body || config.field || config.url || config.userId || config.sequenceId || ''
  return primary ? `${actionLabel(action.type)}: ${primary}` : actionLabel(action.type)
}

function automationSummary(automation) {
  const conditions = automation.conditions || []
  const actions = automation.actions || []
  const conditionText = conditions.length === 0 ? 'no conditions' : `${automation.conditionLogic || 'all'} ${conditions.length} condition${conditions.length === 1 ? '' : 's'}`
  const actionText = actions.length === 0 ? 'no actions' : `${actions.length} action${actions.length === 1 ? '' : 's'}`
  return `${triggerLabel(automation.triggerType)} | ${targetLabel(automation.triggerType, automation.targetEntityType)} | ${conditionText} | ${actionText} | order ${automation.position ?? 0}`
}

export function SettingsAutomationsRoute() {
  const { session } = useAuth()
  usePageTitle('Automations')
  const role = session?.membership?.role || ''
  const canManage = role === 'owner' || role === 'admin'
  const [automations, setAutomations] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [conditionDraft, setConditionDraft] = useState(() => emptyConditionDraft())
  const [actionDraft, setActionDraft] = useState(() => emptyActionDraft())

  async function loadAutomations({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextAutomations = await listWorkflowAutomations({ signal })
      setAutomations(nextAutomations)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load workflow automations.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadAutomations({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
    setConditionDraft(emptyConditionDraft())
    setActionDraft(emptyActionDraft())
  }

  function startEdit(automation) {
    setEditingId(automation.id)
    setForm(formFromAutomation(automation))
    setConditionDraft(emptyConditionDraft(automation.targetEntityType || 'contact'))
    setActionDraft(emptyActionDraft())
    setStatus('')
  }

  function updateTriggerType(triggerType) {
    const targetEntityType = targetOptionsForTrigger(triggerType)[0].value
    setForm({ ...form, triggerType, targetEntityType })
    setConditionDraft(emptyConditionDraft(targetEntityType))
  }

  function updateTargetEntityType(targetEntityType) {
    setForm({ ...form, targetEntityType })
    setConditionDraft(emptyConditionDraft(targetEntityType))
  }

  function addConditionFromBuilder() {
    const field = conditionDraft.field.trim()
    const operator = conditionDraft.operator.trim() || 'equals'
    const value = conditionDraft.value.trim()
    if (!field || (operator !== 'exists' && !value)) {
      setError('Choose a condition field and value before adding it.')
      return
    }
    let currentConditions
    try {
      currentConditions = parseConditions(form.conditionsText)
    } catch (parseError) {
      setError(parseError.message || 'Conditions must be valid JSON before using the builder.')
      return
    }
    const nextConditions = [...currentConditions, { field, operator, value: operator === 'exists' ? '' : value }]
    setForm({ ...form, conditionsText: JSON.stringify(nextConditions, null, 2) })
    setConditionDraft({ ...conditionDraft, value: '' })
    setError('')
  }

  function removeCondition(index) {
    const nextConditions = parseConditionsForBuilder(form.conditionsText).filter((_, currentIndex) => currentIndex !== index)
    setForm({ ...form, conditionsText: JSON.stringify(nextConditions, null, 2) })
  }

  function updateActionType(type) {
    setActionDraft(emptyActionDraft(type))
  }

  function updateActionConfig(key, value) {
    setActionDraft({ ...actionDraft, config: { ...actionDraft.config, [key]: value } })
  }

  function addActionFromBuilder() {
    const configFields = actionConfigFields[actionDraft.type] || []
    const config = Object.fromEntries(configFields.map((field) => [field.key, String(actionDraft.config[field.key] || '').trim()]).filter(([, value]) => value !== ''))
    if (configFields.some((field) => !config[field.key])) {
      setError('Fill in the action details before adding it.')
      return
    }
    let currentActions
    try {
      currentActions = parseActions(form.actionsText)
    } catch (parseError) {
      setError(parseError.message || 'Actions must be valid JSON before using the builder.')
      return
    }
    const nextActions = [...currentActions, { type: actionDraft.type, config }]
    setForm({ ...form, actionsText: JSON.stringify(nextActions, null, 2) })
    setActionDraft(emptyActionDraft(actionDraft.type))
    setError('')
  }

  function removeAction(index) {
    const nextActions = parseActionsForBuilder(form.actionsText).filter((_, currentIndex) => currentIndex !== index)
    setForm({ ...form, actionsText: JSON.stringify(nextActions, null, 2) })
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage) return

    let payload
    try {
      payload = payloadFromForm(form)
    } catch (parseError) {
      setError(parseError.message || 'Trigger config, conditions, and actions must be valid JSON.')
      return
    }

    setIsSaving(true)
    setStatus('')
    try {
      if (editingId) {
        const updated = await updateWorkflowAutomation(editingId, payload)
        setAutomations((current) => current.map((automation) => (automation.id === editingId ? updated : automation)))
        setStatus('Workflow automation updated.')
      } else {
        const created = await createWorkflowAutomation(payload)
        setAutomations((current) => [created, ...current])
        setStatus('Workflow automation created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save workflow automation.')
    } finally {
      setIsSaving(false)
    }
  }

  const targetOptions = targetOptionsForTrigger(form.triggerType)
  const conditionFieldOptions = conditionOptionsForTarget(form.targetEntityType)
  const builderConditions = parseConditionsForBuilder(form.conditionsText)
  const builderActions = parseActionsForBuilder(form.actionsText)
  const selectedActionFields = actionConfigFields[actionDraft.type] || []

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Workflow automations</h2>
              <p>Define automation triggers, conditions, and action plans now; execution, delays, and run history come in later slices.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading workflow automations...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadAutomations()} retryLabel="Retry automations" /> : null}
          <div className="record-list" role="list" aria-label="Workflow automations">
            {!isLoading && automations.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No workflow automations yet.</p>
                  <p className="field-hint">Start by modeling the event, filters, and future actions for automation work.</p>
                </div>
              </article>
            ) : automations.map((automation) => (
              <article className={automation.isActive ? 'record-row' : 'record-row record-row-alert'} key={automation.id} role="listitem">
                <div>
                  <h3>{automation.name}</h3>
                  <p className="field-hint">{automationSummary(automation)}</p>
                  {automation.description ? <p>{automation.description}</p> : null}
                </div>
                <div>
                  <span className="chip">{automation.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(automation)}>Edit</Button> : null}
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit automation trigger' : 'New automation trigger'}</h2>
              <p className="field-hint">This foundation saves trigger, condition, and action definitions; it does not run actions yet.</p>
            </div>
            <Field label="Automation name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="New lead follow-up" required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="Describe when this automation should start." />
            </Field>
            <Field label="Trigger type">
              <select className="text-input" value={form.triggerType} onChange={(event) => updateTriggerType(event.target.value)}>
                {triggerOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Target record">
              <select className="text-input" value={form.targetEntityType} onChange={(event) => updateTargetEntityType(event.target.value)}>
                {targetOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Trigger config JSON" hint='Optional string key/value object, for example {"formPublicId":"lf_public"}.'>
              <textarea className="text-input" rows={5} value={form.triggerConfigText} onChange={(event) => setForm({ ...form, triggerConfigText: event.target.value })} />
            </Field>
            <div className="card-stack" aria-label="Visual workflow builder">
              <div>
                <h3>Visual workflow builder</h3>
                <p className="field-hint">Build the trigger, condition checks, and ordered action plan without hand-editing the JSON arrays.</p>
              </div>
              <div className="record-list" role="list" aria-label="Workflow builder steps">
                <article className="record-row" role="listitem">
                  <div>
                    <p className="field-hint">Step 1: Trigger</p>
                    <p>{triggerLabel(form.triggerType)} on {targetLabel(form.triggerType, form.targetEntityType)}</p>
                  </div>
                </article>
                <article className="record-row" role="listitem">
                  <div>
                    <p className="field-hint">Step 2: Conditions</p>
                    <p>{builderConditions.length === 0 ? 'Run for every matching trigger.' : `${form.conditionLogic === 'any' ? 'Any' : 'All'} of ${builderConditions.length} condition${builderConditions.length === 1 ? '' : 's'} must match.`}</p>
                  </div>
                </article>
                <article className="record-row" role="listitem">
                  <div>
                    <p className="field-hint">Step 3: Actions</p>
                    <p>{builderActions.length === 0 ? 'No actions planned yet.' : `${builderActions.length} action${builderActions.length === 1 ? '' : 's'} will run in order.`}</p>
                  </div>
                </article>
              </div>
            </div>
            <Field label="Condition logic">
              <select className="text-input" value={form.conditionLogic} onChange={(event) => setForm({ ...form, conditionLogic: event.target.value })}>
                <option value="all">All conditions</option>
                <option value="any">Any condition</option>
              </select>
            </Field>
            <div className="card-stack">
              <div className="button-row">
                <Field label="Condition field">
                  <select className="text-input" value={conditionDraft.field} onChange={(event) => setConditionDraft({ ...conditionDraft, field: event.target.value })}>
                    {conditionFieldOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                  </select>
                </Field>
                <Field label="Condition operator">
                  <select className="text-input" value={conditionDraft.operator} onChange={(event) => setConditionDraft({ ...conditionDraft, operator: event.target.value })}>
                    {conditionOperatorOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                  </select>
                </Field>
              </div>
              <Field label="Condition value" hint="Leave blank only when using the exists operator.">
                <input className="text-input" value={conditionDraft.value} onChange={(event) => setConditionDraft({ ...conditionDraft, value: event.target.value })} />
              </Field>
              <Button className="button-secondary" type="button" onClick={addConditionFromBuilder}>Add condition</Button>
              {builderConditions.length > 0 ? (
                <div className="record-list" role="list" aria-label="Builder conditions">
                  {builderConditions.map((condition, index) => (
                    <article className="record-row" role="listitem" key={`${condition.field}-${condition.operator}-${index}`}>
                      <div>
                        <p>{conditionSummary(condition)}</p>
                      </div>
                      <Button className="button-secondary" type="button" onClick={() => removeCondition(index)}>Remove</Button>
                    </article>
                  ))}
                </div>
              ) : null}
            </div>
            <Field label="Conditions JSON" hint='Optional array like [{"field":"status","operator":"equals","value":"lead"}]. Operators: equals, notEquals, contains, exists, greaterThan, lessThan.'>
              <textarea className="text-input" rows={6} value={form.conditionsText} onChange={(event) => setForm({ ...form, conditionsText: event.target.value })} />
            </Field>
            <div className="card-stack">
              <Field label="Action type">
                <select className="text-input" value={actionDraft.type} onChange={(event) => updateActionType(event.target.value)}>
                  {actionOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                </select>
              </Field>
              {selectedActionFields.map((field) => (
                <Field key={field.key} label={field.label}>
                  <input className="text-input" value={actionDraft.config[field.key] || ''} onChange={(event) => updateActionConfig(field.key, event.target.value)} />
                </Field>
              ))}
              <Button className="button-secondary" type="button" onClick={addActionFromBuilder}>Add action</Button>
              {builderActions.length > 0 ? (
                <div className="record-list" role="list" aria-label="Builder actions">
                  {builderActions.map((action, index) => (
                    <article className="record-row" role="listitem" key={`${action.type}-${index}`}>
                      <div>
                        <p>{actionSummary(action)}</p>
                      </div>
                      <Button className="button-secondary" type="button" onClick={() => removeAction(index)}>Remove</Button>
                    </article>
                  ))}
                </div>
              ) : null}
            </div>
            <Field label="Actions JSON" hint='Optional ordered array like [{"type":"create_task","config":{"title":"Call new lead"}}]. Types: update_field, create_task, send_email, send_sms, assign_owner, add_to_sequence, call_webhook, notify.'>
              <textarea className="text-input" rows={7} value={form.actionsText} onChange={(event) => setForm({ ...form, actionsText: event.target.value })} />
            </Field>
            <Field label="Order">
              <input className="text-input" type="number" min="0" value={form.position} onChange={(event) => setForm({ ...form, position: event.target.value })} />
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active trigger definition
            </label>
            <div className="button-row">
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save automation trigger' : 'Create automation trigger'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

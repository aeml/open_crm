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

function targetOptionsForTrigger(triggerType) {
  if (triggerType === 'stage_changed') return [{ value: 'deal', label: 'Deal' }]
  if (triggerType === 'form_submitted') return [{ value: 'lead_form', label: 'Lead form' }]
  if (triggerType === 'inbound_email') return [{ value: 'email_message', label: 'Email message' }]
  if (triggerType === 'webhook') return [{ value: 'webhook', label: 'Webhook' }]
  return recordTargets
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
  }

  function startEdit(automation) {
    setEditingId(automation.id)
    setForm(formFromAutomation(automation))
    setStatus('')
  }

  function updateTriggerType(triggerType) {
    setForm({ ...form, triggerType, targetEntityType: targetOptionsForTrigger(triggerType)[0].value })
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
              <select className="text-input" value={form.targetEntityType} onChange={(event) => setForm({ ...form, targetEntityType: event.target.value })}>
                {targetOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Trigger config JSON" hint='Optional string key/value object, for example {"formPublicId":"lf_public"}.'>
              <textarea className="text-input" rows={5} value={form.triggerConfigText} onChange={(event) => setForm({ ...form, triggerConfigText: event.target.value })} />
            </Field>
            <Field label="Condition logic">
              <select className="text-input" value={form.conditionLogic} onChange={(event) => setForm({ ...form, conditionLogic: event.target.value })}>
                <option value="all">All conditions</option>
                <option value="any">Any condition</option>
              </select>
            </Field>
            <Field label="Conditions JSON" hint='Optional array like [{"field":"status","operator":"equals","value":"lead"}]. Operators: equals, notEquals, contains, exists, greaterThan, lessThan.'>
              <textarea className="text-input" rows={6} value={form.conditionsText} onChange={(event) => setForm({ ...form, conditionsText: event.target.value })} />
            </Field>
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

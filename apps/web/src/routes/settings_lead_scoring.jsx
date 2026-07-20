import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createLeadScoringRule, listLeadScoringRules, updateLeadScoringRule } from '../lib/lead_scoring'
import { listOrganizationUsers } from '../lib/users'
import { usePageTitle } from '../lib/use_page_title'

const fieldOptions = [
  { value: 'status', label: 'Status' },
  { value: 'leadSource', label: 'Lead source' },
  { value: 'utmCampaign', label: 'UTM campaign' },
  { value: 'utmSource', label: 'UTM source' },
  { value: 'utmMedium', label: 'UTM medium' },
  { value: 'jobTitle', label: 'Job title' },
  { value: 'email', label: 'Email' },
  { value: 'emailDomain', label: 'Email domain' },
  { value: 'phone', label: 'Phone' }
]

function emptyForm() {
  return {
    name: '',
    description: '',
    field: 'status',
    operator: 'equals',
    value: 'lead',
    scoreDelta: '10',
    assignToUserId: '',
    isActive: true,
    position: '0'
  }
}

function formFromRule(rule) {
  return {
    name: rule.name || '',
    description: rule.description || '',
    field: rule.field || 'status',
    operator: rule.operator || 'equals',
    value: rule.value || '',
    scoreDelta: String(rule.scoreDelta ?? 0),
    assignToUserId: rule.assignToUserId ? String(rule.assignToUserId) : '',
    isActive: rule.isActive !== false,
    position: String(rule.position ?? 0)
  }
}

function payloadFromForm(form) {
  return {
    name: form.name,
    description: form.description,
    field: form.field,
    operator: form.operator,
    value: form.operator === 'exists' ? '' : form.value,
    scoreDelta: Number.parseInt(String(form.scoreDelta || 0), 10) || 0,
    assignToUserId: Number.parseInt(String(form.assignToUserId || 0), 10) || 0,
    isActive: form.isActive,
    position: Number.parseInt(String(form.position || 0), 10) || 0
  }
}

function userLabel(user) {
  return `${user.firstName || ''} ${user.lastName || ''}`.trim() || user.email
}

function fieldLabel(field) {
  return fieldOptions.find((option) => option.value === field)?.label || field
}

function ruleSummary(rule) {
  const condition = rule.operator === 'exists'
    ? `${fieldLabel(rule.field)} exists`
    : `${fieldLabel(rule.field)} ${rule.operator} ${rule.value}`
  const score = rule.scoreDelta >= 0 ? `+${rule.scoreDelta}` : String(rule.scoreDelta)
  const assignment = rule.assignToUserName ? `assign ${rule.assignToUserName}` : 'no assignment'
  return `${condition} | ${score} points | ${assignment}`
}

export function SettingsLeadScoringRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Lead Scoring')
  const [rules, setRules] = useState([])
  const [users, setUsers] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadData({ signal } = {}) {
    setIsLoading(true)
    try {
      const [nextRules, nextUsers] = await Promise.all([
        listLeadScoringRules({ signal }),
        listOrganizationUsers({ signal })
      ])
      setRules(nextRules)
      setUsers(nextUsers)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load lead scoring rules.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadData({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
  }

  function startEdit(rule) {
    setEditingId(rule.id)
    setForm(formFromRule(rule))
    setStatus('')
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage) return

    setIsSaving(true)
    setStatus('')
    try {
      const payload = payloadFromForm(form)
      if (editingId) {
        const updated = await updateLeadScoringRule(editingId, payload)
        setRules((current) => current.map((rule) => (rule.id === editingId ? updated : rule)))
        setStatus('Lead scoring rule updated.')
      } else {
        const created = await createLeadScoringRule(payload)
        setRules((current) => [created, ...current])
        setStatus('Lead scoring rule created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save lead scoring rule.')
    } finally {
      setIsSaving(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Lead scoring and routing</h2>
              <p>Score leads from contact fields and route unassigned matches to the right owner.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading lead scoring rules...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadData()} retryLabel="Retry lead scoring" /> : null}
          <div className="record-list" role="list" aria-label="Lead scoring rules">
            {!isLoading && rules.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No scoring rules yet.</p>
                  <p className="field-hint">Create rules from source, campaign, title, email, phone, and status signals.</p>
                </div>
              </article>
            ) : rules.map((rule) => (
              <article className={rule.isActive ? 'record-row' : 'record-row record-row-alert'} key={rule.id} role="listitem">
                <div>
                  <h3>{rule.name}</h3>
                  <p className="field-hint">{ruleSummary(rule)}</p>
                  {rule.description ? <p>{rule.description}</p> : null}
                </div>
                <div>
                  <span className="chip">Order {rule.position ?? 0}</span>
                  <span className="chip">{rule.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(rule)}>Edit</Button> : null}
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
              <h2>{editingId ? 'Edit scoring rule' : 'New scoring rule'}</h2>
              <p className="field-hint">Scores clamp between 0 and 100. Assignment only fills unassigned contact owners during evaluation.</p>
            </div>
            <Field label="Rule name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="High-intent demo request" required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="Contacts from demo campaigns with a business email." />
            </Field>
            <Field label="Field">
              <select className="text-input" value={form.field} onChange={(event) => setForm({ ...form, field: event.target.value })}>
                {fieldOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Operator">
              <select className="text-input" value={form.operator} onChange={(event) => setForm({ ...form, operator: event.target.value })}>
                <option value="equals">Equals</option>
                <option value="contains">Contains</option>
                <option value="exists">Exists</option>
              </select>
            </Field>
            <Field label="Rule value" hint={form.operator === 'exists' ? 'Exists rules do not use a value.' : ''}>
              <input className="text-input" value={form.value} onChange={(event) => setForm({ ...form, value: event.target.value })} placeholder="lead, demo, example.com" disabled={form.operator === 'exists'} required={form.operator !== 'exists'} />
            </Field>
            <Field label="Score delta">
              <input className="text-input" type="number" min="-100" max="100" value={form.scoreDelta} onChange={(event) => setForm({ ...form, scoreDelta: event.target.value })} required />
            </Field>
            <Field label="Assign to" hint="Optional. Only unassigned contacts are routed.">
              <select className="text-input" value={form.assignToUserId} onChange={(event) => setForm({ ...form, assignToUserId: event.target.value })}>
                <option value="">No assignment</option>
                {users.map((user) => <option key={user.id} value={user.id}>{userLabel(user)}</option>)}
              </select>
            </Field>
            <Field label="Order">
              <input className="text-input" type="number" min="0" value={form.position} onChange={(event) => setForm({ ...form, position: event.target.value })} />
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active rule
            </label>
            <div className="button-row">
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save scoring rule' : 'Create scoring rule'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

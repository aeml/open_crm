import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createLeadAudience, listLeadAudiences, previewLeadAudience, updateLeadAudience } from '../lib/lead_audiences'
import { usePageTitle } from '../lib/use_page_title'

function emptyForm() {
  return {
    name: '',
    description: '',
    filters: {
      status: 'lead',
      leadSource: '',
      utmCampaign: '',
      utmSource: '',
      hasEmail: ''
    },
    isActive: true
  }
}

function formFromAudience(audience) {
  const filters = audience.filters || {}
  return {
    name: audience.name || '',
    description: audience.description || '',
    filters: {
      status: filters.status || '',
      leadSource: filters.leadSource || '',
      utmCampaign: filters.utmCampaign || '',
      utmSource: filters.utmSource || '',
      hasEmail: filters.hasEmail || ''
    },
    isActive: audience.isActive !== false
  }
}

function compactFilters(filters = {}) {
  return Object.fromEntries(Object.entries(filters).filter(([, value]) => String(value || '').trim() !== ''))
}

function audiencePayload(form) {
  return {
    name: form.name,
    description: form.description,
    filters: compactFilters(form.filters),
    isActive: form.isActive
  }
}

function filterSummary(filters = {}) {
  const parts = []
  if (filters.status) parts.push(`status ${filters.status}`)
  if (filters.leadSource) parts.push(`source ${filters.leadSource}`)
  if (filters.utmCampaign) parts.push(`campaign ${filters.utmCampaign}`)
  if (filters.utmSource) parts.push(`UTM source ${filters.utmSource}`)
  if (filters.hasEmail) parts.push(filters.hasEmail === 'true' ? 'has email' : 'missing email')
  return parts.join(' | ') || 'All contacts'
}

export function SettingsLeadAudiencesRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Lead Audiences')
  const [audiences, setAudiences] = useState([])
  const [maxAudiences, setMaxAudiences] = useState(100)
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [previewCount, setPreviewCount] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isPreviewing, setIsPreviewing] = useState(false)

  async function loadAudiences({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextCatalog = await listLeadAudiences({ signal })
      setAudiences(nextCatalog.audiences)
      setMaxAudiences(nextCatalog.capacity.maxAudiences)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load lead audiences.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadAudiences({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
    setPreviewCount(null)
  }

  function startEdit(audience) {
    setEditingId(audience.id)
    setForm(formFromAudience(audience))
    setPreviewCount(audience.memberCount ?? null)
    setStatus('')
  }

  function updateFilter(key, value) {
    setForm((current) => ({ ...current, filters: { ...current.filters, [key]: value } }))
    setPreviewCount(null)
  }

  async function handlePreview() {
    setIsPreviewing(true)
    setStatus('')
    try {
      const preview = await previewLeadAudience(compactFilters(form.filters))
      setPreviewCount(preview?.memberCount ?? 0)
      setError('')
    } catch (previewError) {
      setError(previewError.message || 'Unable to preview lead audience.')
    } finally {
      setIsPreviewing(false)
    }
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage) return

    setIsSaving(true)
    setStatus('')
    try {
      const payload = audiencePayload(form)
      if (editingId) {
        const updated = await updateLeadAudience(editingId, payload)
        setAudiences((current) => current.map((audience) => (audience.id === editingId ? updated : audience)))
        setStatus('Lead audience updated.')
      } else {
        const created = await createLeadAudience(payload)
        setAudiences((current) => [created, ...current])
        setStatus('Lead audience created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save lead audience.')
    } finally {
      setIsSaving(false)
    }
  }

  const createAtCapacity = editingId === null && audiences.length >= maxAudiences

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Lead audiences</h2>
              <p>Save dynamic contact segments for campaigns, nurture flows, and follow-up lists.</p>
              <p className="field-hint">{audiences.length} of {maxAudiences} stored audiences.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading lead audiences...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadAudiences()} retryLabel="Retry audiences" /> : null}
          <div className="record-list" role="list" aria-label="Lead audiences">
            {!isLoading && audiences.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No audiences yet.</p>
                  <p className="field-hint">Create an audience from source, campaign, status, and email availability filters.</p>
                </div>
              </article>
            ) : audiences.map((audience) => (
              <article className={audience.isActive ? 'record-row' : 'record-row record-row-alert'} key={audience.id} role="listitem">
                <div>
                  <h3>{audience.name}</h3>
                  <p className="field-hint">{filterSummary(audience.filters)}</p>
                  {audience.description ? <p>{audience.description}</p> : null}
                </div>
                <div>
                  <span className="chip">{audience.memberCount || 0} members</span>
                  <span className="chip">{audience.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(audience)}>Edit</Button> : null}
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
              <h2>{editingId ? 'Edit audience' : 'New audience'}</h2>
              <p className="field-hint">Audiences are dynamic: member counts update as contacts match these filters.</p>
            </div>
            <Field label="Name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Spring demo leads" maxLength={120} required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="Contacts captured from the spring demo campaign." maxLength={1000} />
            </Field>
            <Field label="Status">
              <select className="text-input" value={form.filters.status} onChange={(event) => updateFilter('status', event.target.value)}>
                <option value="">Any status</option>
                <option value="lead">Lead</option>
                <option value="prospect">Prospect</option>
                <option value="customer">Customer</option>
              </select>
            </Field>
            <Field label="Lead source">
              <input className="text-input" value={form.filters.leadSource} onChange={(event) => updateFilter('leadSource', event.target.value)} placeholder="Website form" maxLength={120} />
            </Field>
            <Field label="UTM campaign">
              <input className="text-input" value={form.filters.utmCampaign} onChange={(event) => updateFilter('utmCampaign', event.target.value)} placeholder="spring-demo" maxLength={120} />
            </Field>
            <Field label="UTM source">
              <input className="text-input" value={form.filters.utmSource} onChange={(event) => updateFilter('utmSource', event.target.value)} placeholder="google" maxLength={120} />
            </Field>
            <Field label="Email availability">
              <select className="text-input" value={form.filters.hasEmail} onChange={(event) => updateFilter('hasEmail', event.target.value)}>
                <option value="">Any</option>
                <option value="true">Has email</option>
                <option value="false">Missing email</option>
              </select>
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active audience
            </label>
            <div className="button-row">
              <Button className="button-secondary" type="button" onClick={handlePreview} disabled={isPreviewing}>{isPreviewing ? 'Previewing...' : 'Preview count'}</Button>
              <Button type="submit" disabled={isSaving || createAtCapacity}>{isSaving ? 'Saving...' : editingId ? 'Save audience' : 'Create audience'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
            {previewCount !== null ? <p className="inline-note" role="status">{previewCount} matching contacts.</p> : null}
          </form>
        </Card>
      ) : null}
    </section>
  )
}

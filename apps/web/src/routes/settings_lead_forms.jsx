import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createLeadCaptureForm, listLeadCaptureForms, publicLeadCaptureFormSubmitURL, updateLeadCaptureForm } from '../lib/lead_forms'
import { usePageTitle } from '../lib/use_page_title'

const defaultFields = [
  { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
  { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
  { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' },
  { key: 'phone', label: 'Phone', fieldType: 'tel', required: false, mapTo: 'phone' },
  { key: 'message', label: 'How can we help?', fieldType: 'textarea', required: false, mapTo: '' }
]

function emptyForm() {
  return {
    name: '',
    slug: '',
    title: '',
    description: '',
    successMessage: 'Thanks. We will be in touch soon.',
    sourceLabel: 'Lead capture form',
    isActive: true,
    fields: defaultFields.map((field) => ({ ...field }))
  }
}

function formFromLeadForm(form) {
  return {
    name: form.name || '',
    slug: form.slug || '',
    title: form.title || form.name || '',
    description: form.description || '',
    successMessage: form.successMessage || 'Thanks. We will be in touch soon.',
    sourceLabel: form.sourceLabel || 'Lead capture form',
    isActive: form.isActive !== false,
    fields: (form.fields && form.fields.length > 0 ? form.fields : defaultFields).map((field) => ({ ...field }))
  }
}

function leadFormPayload(form) {
  return {
    name: form.name,
    slug: form.slug,
    title: form.title,
    description: form.description,
    successMessage: form.successMessage,
    sourceLabel: form.sourceLabel,
    isActive: form.isActive,
    fields: form.fields
  }
}

function fieldInputType(field) {
  if (field.fieldType === 'email') return 'email'
  if (field.fieldType === 'tel') return 'tel'
  if (field.fieldType === 'hidden') return 'hidden'
  return 'text'
}

function embedSnippet(form) {
  const action = publicLeadCaptureFormSubmitURL(form.publicId || '')
  const controls = (form.fields || defaultFields).map((field) => {
    if (field.fieldType === 'textarea') {
      return `  <label>${field.label}\n    <textarea name="${field.key}"${field.required ? ' required' : ''}></textarea>\n  </label>`
    }
    return `  <label>${field.label}\n    <input name="${field.key}" type="${fieldInputType(field)}"${field.required ? ' required' : ''}>\n  </label>`
  })

  return [
    `<form method="post" action="${action}">`,
    ...controls,
    '  <input type="hidden" name="sourceUrl" value="https://example.com/contact?utm_source=google&utm_medium=cpc&utm_campaign=spring-demo">',
    `  <input type="hidden" name="leadSource" value="${form.sourceLabel || 'Lead capture form'}">`,
    '  <input type="hidden" name="utm_source" value="">',
    '  <input type="hidden" name="utm_medium" value="">',
    '  <input type="hidden" name="utm_campaign" value="">',
    '  <input type="hidden" name="utm_term" value="">',
    '  <input type="hidden" name="utm_content" value="">',
    '  <button type="submit">Submit</button>',
    '</form>'
  ].join('\n')
}

function mappedFieldLabel(field) {
  return field.mapTo ? `Maps to contact ${field.mapTo}` : 'Stored on the lead form submission'
}

export function SettingsLeadFormsRoute() {
  const { session, canAdminister: canManage } = useAuth()
  usePageTitle('Lead Forms')
  const [forms, setForms] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadForms({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextForms = await listLeadCaptureForms({ signal })
      setForms(nextForms)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load lead forms.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadForms({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
  }

  function startEdit(nextForm) {
    setEditingId(nextForm.id)
    setForm(formFromLeadForm(nextForm))
    setStatus('')
  }

  function updateField(index, patch) {
    setForm((current) => ({
      ...current,
      fields: current.fields.map((field, fieldIndex) => (fieldIndex === index ? { ...field, ...patch } : field))
    }))
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage) return

    setIsSaving(true)
    setStatus('')
    try {
      const payload = leadFormPayload(form)
      if (editingId) {
        const updated = await updateLeadCaptureForm(editingId, payload)
        setForms((current) => current.map((item) => (item.id === editingId ? updated : item)))
        setStatus('Lead form updated.')
      } else {
        const created = await createLeadCaptureForm(payload)
        setForms((current) => [created, ...current])
        setStatus('Lead form created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save lead form.')
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
              <h2>Lead forms</h2>
              <p>Create embeddable forms that capture website inquiries as CRM lead contacts for {session?.organization?.name || 'your team'}.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading lead forms...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadForms()} retryLabel="Retry lead forms" /> : null}
          <div className="record-list" role="list" aria-label="Lead forms">
            {!isLoading && forms.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No lead forms yet.</p>
                  <p className="field-hint">Create a website form to start converting inbound demand into lead contacts.</p>
                </div>
              </article>
            ) : forms.map((item) => (
              <article className={item.isActive ? 'record-row' : 'record-row record-row-alert'} key={item.id} role="listitem">
                <div>
                  <h3>{item.name}</h3>
                  <p className="field-hint">/{item.slug} · {item.submissionCount || 0} submissions · public id {item.publicId}</p>
                  {item.description ? <p className="field-hint">{item.description}</p> : null}
                  {item.publicId ? <textarea className="text-input" readOnly rows={8} aria-label={`Embed code for ${item.name}`} value={embedSnippet(item)} /> : null}
                </div>
                <div>
                  <span className="chip">{item.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(item)}>Edit</Button> : null}
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
              <h2>{editingId ? 'Edit lead form' : 'New lead form'}</h2>
              <p className="field-hint">This first form builder keeps mappings fixed to standard contact fields and stores extra message text on the submission.</p>
            </div>
            <Field label="Name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Website contact form" required />
            </Field>
            <Field label="Slug" hint="Optional. Used for admin readability; the public embed uses a generated public id.">
              <input className="text-input" value={form.slug} onChange={(event) => setForm({ ...form, slug: event.target.value })} placeholder="website-contact" />
            </Field>
            <Field label="Title">
              <input className="text-input" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Talk to our team" />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="Tell visitors what happens after they submit." />
            </Field>
            <Field label="Success message">
              <input className="text-input" value={form.successMessage} onChange={(event) => setForm({ ...form, successMessage: event.target.value })} required />
            </Field>
            <Field label="Source label">
              <input className="text-input" value={form.sourceLabel} onChange={(event) => setForm({ ...form, sourceLabel: event.target.value })} required />
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active lead form
            </label>
            <div className="record-list" role="list" aria-label="Mapped lead form fields">
              {form.fields.map((field, index) => (
                <article className="record-row" key={field.key} role="listitem">
                  <div>
                    <Field label={`${field.key} label`} hint={mappedFieldLabel(field)}>
                      <input className="text-input" value={field.label} onChange={(event) => updateField(index, { label: event.target.value })} required />
                    </Field>
                  </div>
                  <div>
                    <label className="field-hint">
                      <input type="checkbox" checked={field.required} onChange={(event) => updateField(index, { required: event.target.checked })} /> Required
                    </label>
                  </div>
                </article>
              ))}
            </div>
            <div>
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save lead form' : 'Create lead form'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

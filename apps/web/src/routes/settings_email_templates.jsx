import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listEmailTemplates, createEmailTemplate, updateEmailTemplate, deleteEmailTemplate } from '../lib/email_templates'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = { name: '', subject: '', body: '' }

export function SettingsEmailTemplatesRoute() {
  const { session } = useAuth()
  usePageTitle('Email Templates')
  const role = session?.membership?.role || ''
  const canManage = role !== 'viewer'
  const [templates, setTemplates] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadTemplates({ signal } = {}) {
    setIsLoading(true)
    try {
      const next = await listEmailTemplates({ signal })
      setTemplates(next)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email templates.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadTemplates({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setForm(emptyForm)
    setEditingId(null)
  }

  function startEdit(template) {
    setEditingId(template.id)
    setForm({ name: template.name, subject: template.subject, body: template.body })
  }

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSaving(true)
    try {
      if (editingId) {
        const updated = await updateEmailTemplate(editingId, form)
        setTemplates((current) => current.map((t) => (t.id === editingId ? updated : t)))
      } else {
        const created = await createEmailTemplate(form)
        setTemplates((current) => [...current, created])
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email template.')
    } finally {
      setIsSaving(false)
    }
  }

  async function handleDelete(templateId) {
    try {
      await deleteEmailTemplate(templateId)
      setTemplates((current) => current.filter((t) => t.id !== templateId))
      if (editingId === templateId) {
        resetForm()
      }
      setError('')
    } catch (deleteError) {
      setError(deleteError.message || 'Unable to delete email template.')
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Email templates</h2>
              <p>Reusable messages for {session?.organization?.name || 'your team'}. Use merge fields like {'{{first_name}}'} that fill in when you send.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading templates...</p> : null}
          {error ? <InlineError message={error} /> : null}
          <div className="record-list" role="list" aria-label="Email templates">
            {!isLoading && templates.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No email templates yet.</p>
                  <p className="field-hint">Create your first template to reuse common messages.</p>
                </div>
              </article>
            ) : templates.map((template) => (
              <article className="record-row" key={template.id} role="listitem">
                <div>
                  <h3>{template.name}</h3>
                  <p className="field-hint">{template.subject}</p>
                </div>
                {canManage ? (
                  <div>
                    <Button className="button-secondary" type="button" onClick={() => startEdit(template)}>Edit</Button>
                    <Button className="button-secondary" type="button" onClick={() => handleDelete(template.id)}>Delete</Button>
                  </div>
                ) : null}
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <h2>{editingId ? 'Edit template' : 'New template'}</h2>
            <Field label="Name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required />
            </Field>
            <Field label="Subject">
              <input className="text-input" value={form.subject} onChange={(event) => setForm({ ...form, subject: event.target.value })} required />
            </Field>
            <Field label="Body">
              <textarea className="text-input" rows={8} value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} required />
            </Field>
            <div>
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save changes' : 'Create template'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

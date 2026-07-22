import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { MergeFieldCatalog } from '../components/merge_field_catalog'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { listEmailTemplates, listEmailTemplateMergeFields, listEmailSnippets, createEmailTemplate, updateEmailTemplate, deleteEmailTemplate, createEmailSnippet, updateEmailSnippet, deleteEmailSnippet } from '../lib/email_templates'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = { name: '', subject: '', body: '' }
const emptySnippetForm = { name: '', body: '' }

export function SettingsEmailTemplatesRoute() {
  const { session, canWrite: canManage } = useAuth()
  usePageTitle('Email Templates')
  const [templates, setTemplates] = useState([])
  const [snippets, setSnippets] = useState([])
  const [mergeFieldGroups, setMergeFieldGroups] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [snippetForm, setSnippetForm] = useState(emptySnippetForm)
  const [editingId, setEditingId] = useState(null)
  const [editingSnippetId, setEditingSnippetId] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isSavingSnippet, setIsSavingSnippet] = useState(false)

  async function loadTemplates({ signal } = {}) {
    setIsLoading(true)
    try {
      const [next, fields, nextSnippets] = await Promise.all([
        listEmailTemplates({ signal }),
        listEmailTemplateMergeFields({ signal }),
        listEmailSnippets({ signal })
      ])
      setTemplates(next)
      setMergeFieldGroups(fields)
      setSnippets(nextSnippets)
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

  function resetSnippetForm() {
    setSnippetForm(emptySnippetForm)
    setEditingSnippetId(null)
  }

  function startEdit(template) {
    setEditingId(template.id)
    setForm({ name: template.name, subject: template.subject, body: template.body })
  }

  function startSnippetEdit(snippet) {
    setEditingSnippetId(snippet.id)
    setSnippetForm({ name: snippet.name, body: snippet.body })
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

  async function handleSnippetSubmit(event) {
    event.preventDefault()
    setIsSavingSnippet(true)
    try {
      if (editingSnippetId) {
        const updated = await updateEmailSnippet(editingSnippetId, snippetForm)
        setSnippets((current) => current.map((snippet) => (snippet.id === editingSnippetId ? updated : snippet)))
      } else {
        const created = await createEmailSnippet(snippetForm)
        setSnippets((current) => [...current, created])
      }
      resetSnippetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save email snippet.')
    } finally {
      setIsSavingSnippet(false)
    }
  }

  async function handleSnippetDelete(snippetId) {
    try {
      await deleteEmailSnippet(snippetId)
      setSnippets((current) => current.filter((snippet) => snippet.id !== snippetId))
      if (editingSnippetId === snippetId) {
        resetSnippetForm()
      }
      setError('')
    } catch (deleteError) {
      setError(deleteError.message || 'Unable to delete email snippet.')
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Email templates</h2>
              <p>Reusable messages for {session?.organization?.name || 'your team'}. Preview current record values and send a private test to yourself from the record composer.</p>
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
          {mergeFieldGroups.length > 0 ? (
            <div className="card-stack">
              <div>
                <h3>Available merge fields</h3>
                <p className="field-hint">Use these tokens in template subjects and bodies. Active contact and company custom fields use a collision-safe custom namespace.</p>
              </div>
              <MergeFieldCatalog groups={mergeFieldGroups} />
            </div>
          ) : null}
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Email snippets</h2>
              <p>Reusable body fragments your team can insert into one-to-one email drafts.</p>
            </div>
          </div>
          <div className="record-list" role="list" aria-label="Email snippets">
            {!isLoading && snippets.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No email snippets yet.</p>
                  <p className="field-hint">Create short answers, CTAs, or scheduling blocks for reuse.</p>
                </div>
              </article>
            ) : snippets.map((snippet) => (
              <article className="record-row" key={snippet.id} role="listitem">
                <div>
                  <h3>{snippet.name}</h3>
                  <p className="field-hint">{snippet.body}</p>
                </div>
                {canManage ? (
                  <div>
                    <Button className="button-secondary" type="button" onClick={() => startSnippetEdit(snippet)}>Edit</Button>
                    <Button className="button-secondary" type="button" onClick={() => handleSnippetDelete(snippet.id)}>Delete</Button>
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

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSnippetSubmit}>
            <h2>{editingSnippetId ? 'Edit snippet' : 'New snippet'}</h2>
            <Field label="Snippet name">
              <input className="text-input" value={snippetForm.name} onChange={(event) => setSnippetForm({ ...snippetForm, name: event.target.value })} required />
            </Field>
            <Field label="Snippet body">
              <textarea className="text-input" rows={5} value={snippetForm.body} onChange={(event) => setSnippetForm({ ...snippetForm, body: event.target.value })} required />
            </Field>
            <div>
              <Button type="submit" disabled={isSavingSnippet}>{isSavingSnippet ? 'Saving...' : editingSnippetId ? 'Save snippet' : 'Create snippet'}</Button>
              {editingSnippetId ? <Button className="button-secondary" type="button" onClick={resetSnippetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

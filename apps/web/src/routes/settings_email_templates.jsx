import { useEffect, useRef, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { MergeFieldCatalog } from '../components/merge_field_catalog'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import {
  createEmailSnippet,
  createEmailTemplate,
  deleteEmailSnippet,
  deleteEmailTemplate,
  listEmailSnippetPage,
  listEmailTemplateMergeFields,
  listEmailTemplatePage,
  updateEmailSnippet,
  updateEmailTemplate
} from '../lib/email_templates'
import { usePageTitle } from '../lib/use_page_title'

const pageSize = 50
const emptyMeta = { page: 1, pageSize, total: 0 }
const emptyForm = { name: '', subject: '', body: '', expectedRevision: 0 }
const emptySnippetForm = { name: '', body: '', expectedRevision: 0 }

function formFromTemplate(template) {
  return {
    name: template.name,
    subject: template.subject,
    body: template.body,
    expectedRevision: template.revision
  }
}

function formFromSnippet(snippet) {
  return {
    name: snippet.name,
    body: snippet.body,
    expectedRevision: snippet.revision
  }
}

export function SettingsEmailTemplatesRoute() {
  const { session, canWrite: canManage } = useAuth()
  usePageTitle('Email Templates')
  const [templates, setTemplates] = useState([])
  const [templateMeta, setTemplateMeta] = useState(emptyMeta)
  const [snippets, setSnippets] = useState([])
  const [snippetMeta, setSnippetMeta] = useState(emptyMeta)
  const [mergeFieldGroups, setMergeFieldGroups] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [snippetForm, setSnippetForm] = useState(emptySnippetForm)
  const [editingId, setEditingId] = useState(null)
  const [editingSnippetId, setEditingSnippetId] = useState(null)
  const [templateError, setTemplateError] = useState('')
  const [snippetError, setSnippetError] = useState('')
  const [mergeFieldError, setMergeFieldError] = useState('')
  const [templateStatus, setTemplateStatus] = useState('')
  const [snippetStatus, setSnippetStatus] = useState('')
  const [isLoadingTemplates, setIsLoadingTemplates] = useState(true)
  const [isLoadingSnippets, setIsLoadingSnippets] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [isSavingSnippet, setIsSavingSnippet] = useState(false)
  const [deletingTemplateId, setDeletingTemplateId] = useState(null)
  const [deletingSnippetId, setDeletingSnippetId] = useState(null)
  const [templateSearchInput, setTemplateSearchInput] = useState('')
  const [templateSearch, setTemplateSearch] = useState('')
  const [templatePage, setTemplatePage] = useState(1)
  const [snippetSearchInput, setSnippetSearchInput] = useState('')
  const [snippetSearch, setSnippetSearch] = useState('')
  const [snippetPage, setSnippetPage] = useState(1)
  const latestTemplateLoad = useRef(0)
  const latestSnippetLoad = useRef(0)
  const operationPending = useRef(false)
  const mutationPending = isSaving || isSavingSnippet || deletingTemplateId !== null || deletingSnippetId !== null

  async function loadTemplates({ signal, requestedPage = templatePage, search = templateSearch } = {}) {
    const loadId = latestTemplateLoad.current + 1
    latestTemplateLoad.current = loadId
    setIsLoadingTemplates(true)
    try {
      const catalog = await listEmailTemplatePage({ search, page: requestedPage, pageSize, signal })
      if (signal?.aborted || loadId !== latestTemplateLoad.current) return null
      setTemplates(catalog.templates)
      setTemplateMeta(catalog.meta)
      setTemplateError('')
      return catalog
    } catch (loadError) {
      if (!isAbortError(loadError) && loadId === latestTemplateLoad.current) {
        setTemplateError(loadError.message || 'Unable to load email templates.')
      }
      return null
    } finally {
      if (!signal?.aborted && loadId === latestTemplateLoad.current) setIsLoadingTemplates(false)
    }
  }

  async function loadSnippets({ signal, requestedPage = snippetPage, search = snippetSearch } = {}) {
    const loadId = latestSnippetLoad.current + 1
    latestSnippetLoad.current = loadId
    setIsLoadingSnippets(true)
    try {
      const catalog = await listEmailSnippetPage({ search, page: requestedPage, pageSize, signal })
      if (signal?.aborted || loadId !== latestSnippetLoad.current) return null
      setSnippets(catalog.snippets)
      setSnippetMeta(catalog.meta)
      setSnippetError('')
      return catalog
    } catch (loadError) {
      if (!isAbortError(loadError) && loadId === latestSnippetLoad.current) {
        setSnippetError(loadError.message || 'Unable to load email snippets.')
      }
      return null
    } finally {
      if (!signal?.aborted && loadId === latestSnippetLoad.current) setIsLoadingSnippets(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadTemplates({ signal: controller.signal })
    return () => controller.abort()
  }, [templatePage, templateSearch])

  useEffect(() => {
    const controller = new AbortController()
    loadSnippets({ signal: controller.signal })
    return () => controller.abort()
  }, [snippetPage, snippetSearch])

  useEffect(() => {
    const controller = new AbortController()
    listEmailTemplateMergeFields({ signal: controller.signal })
      .then((groups) => {
        if (!controller.signal.aborted) {
          setMergeFieldGroups(groups)
          setMergeFieldError('')
        }
      })
      .catch((loadError) => {
        if (!isAbortError(loadError)) setMergeFieldError(loadError.message || 'Unable to load merge fields.')
      })
    return () => controller.abort()
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
    setForm(formFromTemplate(template))
  }

  function startSnippetEdit(snippet) {
    setEditingSnippetId(snippet.id)
    setSnippetForm(formFromSnippet(snippet))
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (operationPending.current) return
    operationPending.current = true
    setIsSaving(true)
    setTemplateStatus('')
    try {
      const operation = editingId ? 'updated' : 'created'
      let createdName = ''
      if (editingId) await updateEmailTemplate(editingId, form)
      else {
        const created = await createEmailTemplate({ name: form.name, subject: form.subject, body: form.body })
        createdName = created.name
      }
      resetForm()
      setTemplateStatus(`Email template ${operation}.`)
      setTemplateError('')
      if (createdName) {
        setTemplateSearchInput(createdName)
        if (templatePage === 1 && templateSearch === createdName) await loadTemplates({ requestedPage: 1, search: createdName })
        else {
          setTemplatePage(1)
          setTemplateSearch(createdName)
        }
      } else if (templatePage === 1) await loadTemplates({ requestedPage: 1 })
      else setTemplatePage(1)
    } catch (saveError) {
      setTemplateError(saveError.message || 'Unable to save email template.')
    } finally {
      setIsSaving(false)
      operationPending.current = false
    }
  }

  async function handleDelete(template) {
    if (operationPending.current) return
    operationPending.current = true
    setDeletingTemplateId(template.id)
    setTemplateStatus('')
    try {
      await deleteEmailTemplate(template.id, template.revision)
      if (editingId === template.id) resetForm()
      setTemplateStatus('Email template deleted.')
      setTemplateError('')
      const catalog = await loadTemplates()
      if (catalog && catalog.templates.length === 0 && catalog.meta.total > 0 && templatePage > 1) {
        setTemplatePage((current) => current - 1)
      }
    } catch (deleteError) {
      setTemplateError(deleteError.message || 'Unable to delete email template.')
    } finally {
      setDeletingTemplateId(null)
      operationPending.current = false
    }
  }

  async function handleSnippetSubmit(event) {
    event.preventDefault()
    if (operationPending.current) return
    operationPending.current = true
    setIsSavingSnippet(true)
    setSnippetStatus('')
    try {
      const operation = editingSnippetId ? 'updated' : 'created'
      let createdName = ''
      if (editingSnippetId) await updateEmailSnippet(editingSnippetId, snippetForm)
      else {
        const created = await createEmailSnippet({ name: snippetForm.name, body: snippetForm.body })
        createdName = created.name
      }
      resetSnippetForm()
      setSnippetStatus(`Email snippet ${operation}.`)
      setSnippetError('')
      if (createdName) {
        setSnippetSearchInput(createdName)
        if (snippetPage === 1 && snippetSearch === createdName) await loadSnippets({ requestedPage: 1, search: createdName })
        else {
          setSnippetPage(1)
          setSnippetSearch(createdName)
        }
      } else if (snippetPage === 1) await loadSnippets({ requestedPage: 1 })
      else setSnippetPage(1)
    } catch (saveError) {
      setSnippetError(saveError.message || 'Unable to save email snippet.')
    } finally {
      setIsSavingSnippet(false)
      operationPending.current = false
    }
  }

  async function handleSnippetDelete(snippet) {
    if (operationPending.current) return
    operationPending.current = true
    setDeletingSnippetId(snippet.id)
    setSnippetStatus('')
    try {
      await deleteEmailSnippet(snippet.id, snippet.revision)
      if (editingSnippetId === snippet.id) resetSnippetForm()
      setSnippetStatus('Email snippet deleted.')
      setSnippetError('')
      const catalog = await loadSnippets()
      if (catalog && catalog.snippets.length === 0 && catalog.meta.total > 0 && snippetPage > 1) {
        setSnippetPage((current) => current - 1)
      }
    } catch (deleteError) {
      setSnippetError(deleteError.message || 'Unable to delete email snippet.')
    } finally {
      setDeletingSnippetId(null)
      operationPending.current = false
    }
  }

  function applyTemplateSearch(event) {
    event.preventDefault()
    const nextSearch = templateSearchInput.trim()
    if (nextSearch === templateSearch && templatePage === 1) loadTemplates({ requestedPage: 1, search: nextSearch })
    else {
      setTemplatePage(1)
      setTemplateSearch(nextSearch)
    }
  }

  function applySnippetSearch(event) {
    event.preventDefault()
    const nextSearch = snippetSearchInput.trim()
    if (nextSearch === snippetSearch && snippetPage === 1) loadSnippets({ requestedPage: 1, search: nextSearch })
    else {
      setSnippetPage(1)
      setSnippetSearch(nextSearch)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header"><div><h2>Email templates</h2><p>Reusable messages for {session?.organization?.name || 'your team'}. Preview current record values and send a private test to yourself from the record composer.</p></div></div>
          {templateStatus ? <p className="field-hint" role="status">{templateStatus}</p> : null}
          {isLoadingTemplates ? <p className="field-hint" role="status">Loading templates…</p> : null}
          {templateError ? <InlineError message={templateError} /> : null}
          <form className="filters-grid" onSubmit={applyTemplateSearch}>
            <Field label="Search email templates"><input className="text-input" maxLength={100} value={templateSearchInput} disabled={mutationPending} onChange={(event) => setTemplateSearchInput(event.target.value)} placeholder="Template name" /></Field>
            <Button className="button-secondary" type="submit" disabled={isLoadingTemplates || mutationPending}>Apply template search</Button>
          </form>
          <div className="record-list" role="list" aria-label="Email templates">
            {!isLoadingTemplates && templates.length === 0 ? <article className="record-row" role="listitem"><div><p>{templateSearch ? 'No email templates match this search.' : 'No email templates yet.'}</p><p className="field-hint">{templateSearch ? 'Change the search and try again.' : 'Create your first template to reuse common messages.'}</p></div></article> : null}
            {templates.map((template) => <article className="record-row" key={template.id} role="listitem"><div><h3>{template.name}</h3><p className="field-hint">{template.subject} · revision {template.revision}</p></div>{canManage ? <div><Button className="button-secondary" type="button" disabled={mutationPending} onClick={() => startEdit(template)}>Edit</Button><Button className="button-danger" type="button" disabled={mutationPending} onClick={() => handleDelete(template)}>{deletingTemplateId === template.id ? 'Deleting…' : 'Delete'}</Button></div> : null}</article>)}
          </div>
          <p className="field-hint" role="status">Showing {templates.length} of {templateMeta.total} email templates{templateSearch ? ` matching “${templateSearch}”` : ''}. Up to 100 templates may be stored.</p>
          <div className="button-row"><Button className="button-secondary" type="button" disabled={isLoadingTemplates || templatePage <= 1 || mutationPending} onClick={() => setTemplatePage((current) => current - 1)}>Previous template page</Button><Button className="button-secondary" type="button" disabled={isLoadingTemplates || templatePage * templateMeta.pageSize >= templateMeta.total || mutationPending} onClick={() => setTemplatePage((current) => current + 1)}>Next template page</Button></div>
          {mergeFieldError ? <InlineError message={mergeFieldError} /> : null}
          {mergeFieldGroups.length > 0 ? <div className="card-stack"><div><h3>Available merge fields</h3><p className="field-hint">Use these tokens in template subjects and bodies. Active contact and company custom fields use a collision-safe custom namespace.</p></div><MergeFieldCatalog groups={mergeFieldGroups} /></div> : null}
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div className="section-header"><div><h2>Email snippets</h2><p>Reusable body fragments your team can insert into one-to-one email drafts.</p></div></div>
          {snippetStatus ? <p className="field-hint" role="status">{snippetStatus}</p> : null}
          {isLoadingSnippets ? <p className="field-hint" role="status">Loading snippets…</p> : null}
          {snippetError ? <InlineError message={snippetError} /> : null}
          <form className="filters-grid" onSubmit={applySnippetSearch}>
            <Field label="Search email snippets"><input className="text-input" maxLength={100} value={snippetSearchInput} disabled={mutationPending} onChange={(event) => setSnippetSearchInput(event.target.value)} placeholder="Snippet name" /></Field>
            <Button className="button-secondary" type="submit" disabled={isLoadingSnippets || mutationPending}>Apply snippet search</Button>
          </form>
          <div className="record-list" role="list" aria-label="Email snippets">
            {!isLoadingSnippets && snippets.length === 0 ? <article className="record-row" role="listitem"><div><p>{snippetSearch ? 'No email snippets match this search.' : 'No email snippets yet.'}</p><p className="field-hint">{snippetSearch ? 'Change the search and try again.' : 'Create short answers, CTAs, or scheduling blocks for reuse.'}</p></div></article> : null}
            {snippets.map((snippet) => <article className="record-row" key={snippet.id} role="listitem"><div><h3>{snippet.name}</h3><p className="field-hint">{snippet.body} · revision {snippet.revision}</p></div>{canManage ? <div><Button className="button-secondary" type="button" disabled={mutationPending} onClick={() => startSnippetEdit(snippet)}>Edit</Button><Button className="button-danger" type="button" disabled={mutationPending} onClick={() => handleSnippetDelete(snippet)}>{deletingSnippetId === snippet.id ? 'Deleting…' : 'Delete'}</Button></div> : null}</article>)}
          </div>
          <p className="field-hint" role="status">Showing {snippets.length} of {snippetMeta.total} email snippets{snippetSearch ? ` matching “${snippetSearch}”` : ''}. Up to 100 snippets may be stored.</p>
          <div className="button-row"><Button className="button-secondary" type="button" disabled={isLoadingSnippets || snippetPage <= 1 || mutationPending} onClick={() => setSnippetPage((current) => current - 1)}>Previous snippet page</Button><Button className="button-secondary" type="button" disabled={isLoadingSnippets || snippetPage * snippetMeta.pageSize >= snippetMeta.total || mutationPending} onClick={() => setSnippetPage((current) => current + 1)}>Next snippet page</Button></div>
        </div>
      </Card>

      {canManage ? <Card><form className="auth-form card-stack" aria-label={editingId ? 'Edit email template' : 'Create email template'} onSubmit={handleSubmit}><h2>{editingId ? 'Edit template' : 'New template'}</h2><Field label="Name"><input className="text-input" maxLength={120} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} required /></Field><Field label="Subject"><input className="text-input" maxLength={500} value={form.subject} onChange={(event) => setForm({ ...form, subject: event.target.value })} required /></Field><Field label="Body"><textarea className="text-input" maxLength={10000} rows={8} value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} required /></Field><div className="button-row"><Button type="submit" disabled={mutationPending}>{isSaving ? 'Saving…' : editingId ? 'Save changes' : 'Create template'}</Button>{editingId ? <Button className="button-secondary" type="button" disabled={mutationPending} onClick={resetForm}>Cancel</Button> : null}</div></form></Card> : null}

      {canManage ? <Card><form className="auth-form card-stack" aria-label={editingSnippetId ? 'Edit email snippet' : 'Create email snippet'} onSubmit={handleSnippetSubmit}><h2>{editingSnippetId ? 'Edit snippet' : 'New snippet'}</h2><Field label="Snippet name"><input className="text-input" maxLength={120} value={snippetForm.name} onChange={(event) => setSnippetForm({ ...snippetForm, name: event.target.value })} required /></Field><Field label="Snippet body"><textarea className="text-input" maxLength={10000} rows={5} value={snippetForm.body} onChange={(event) => setSnippetForm({ ...snippetForm, body: event.target.value })} required /></Field><div className="button-row"><Button type="submit" disabled={mutationPending}>{isSavingSnippet ? 'Saving…' : editingSnippetId ? 'Save snippet' : 'Create snippet'}</Button>{editingSnippetId ? <Button className="button-secondary" type="button" disabled={mutationPending} onClick={resetSnippetForm}>Cancel</Button> : null}</div></form></Card> : null}
    </section>
  )
}

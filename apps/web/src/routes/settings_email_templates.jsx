import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
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
import { DefinitionCatalogFilters, DefinitionCatalogPagination, DefinitionTextField, useDefinitionCatalog } from './definition_catalog'

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
  const [mergeFieldGroups, setMergeFieldGroups] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [snippetForm, setSnippetForm] = useState(emptySnippetForm)
  const [editingId, setEditingId] = useState(null)
  const [editingSnippetId, setEditingSnippetId] = useState(null)
  const [mergeFieldError, setMergeFieldError] = useState('')
  const [templateStatus, setTemplateStatus] = useState('')
  const [snippetStatus, setSnippetStatus] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [isSavingSnippet, setIsSavingSnippet] = useState(false)
  const [deletingTemplateId, setDeletingTemplateId] = useState(null)
  const [deletingSnippetId, setDeletingSnippetId] = useState(null)
  const {
    appliedSearch: templateSearch, error: templateError, handleSearch: applyTemplateSearch,
    isLoading: isLoadingTemplates, items: templates, load: loadTemplates, meta: templateMeta,
    operationPending, pageNumber: templatePage, searchInput: templateSearchInput,
    setAppliedSearch: setTemplateSearch, setError: setTemplateError,
    setPageNumber: setTemplatePage, setSearchInput: setTemplateSearchInput
  } = useDefinitionCatalog({
    requestPage: listEmailTemplatePage,
    itemsKey: 'templates',
    loadErrorMessage: 'Unable to load email templates.'
  })
  const {
    appliedSearch: snippetSearch, error: snippetError, handleSearch: applySnippetSearch,
    isLoading: isLoadingSnippets, items: snippets, load: loadSnippets, meta: snippetMeta,
    pageNumber: snippetPage, searchInput: snippetSearchInput,
    setAppliedSearch: setSnippetSearch, setError: setSnippetError,
    setPageNumber: setSnippetPage, setSearchInput: setSnippetSearchInput
  } = useDefinitionCatalog({
    requestPage: listEmailSnippetPage,
    itemsKey: 'snippets',
    loadErrorMessage: 'Unable to load email snippets.'
  })
  const mutationPending = isSaving || isSavingSnippet || deletingTemplateId !== null || deletingSnippetId !== null

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

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header"><div><h2>Email templates</h2><p>Reusable messages for {session?.organization?.name || 'your team'}. Preview current record values and send a private test to yourself from the record composer.</p></div></div>
          {templateStatus ? <p className="field-hint" role="status">{templateStatus}</p> : null}
          {isLoadingTemplates ? <p className="field-hint" role="status">Loading templates…</p> : null}
          {templateError ? <InlineError message={templateError} /> : null}
          <DefinitionCatalogFilters applyLabel="Apply template search" disabled={mutationPending} handleSearch={applyTemplateSearch} isLoading={isLoadingTemplates} searchInput={templateSearchInput} searchLabel="Search email templates" searchPlaceholder="Template name" setSearchInput={setTemplateSearchInput} />
          <div className="record-list" role="list" aria-label="Email templates">
            {!isLoadingTemplates && templates.length === 0 ? <article className="record-row" role="listitem"><div><p>{templateSearch ? 'No email templates match this search.' : 'No email templates yet.'}</p><p className="field-hint">{templateSearch ? 'Change the search and try again.' : 'Create your first template to reuse common messages.'}</p></div></article> : null}
            {templates.map((template) => <article className="record-row" key={template.id} role="listitem"><div><h3>{template.name}</h3><p className="field-hint">{template.subject} · revision {template.revision}</p></div>{canManage ? <div><Button className="button-secondary" type="button" disabled={mutationPending} onClick={() => startEdit(template)}>Edit</Button><Button className="button-danger" type="button" disabled={mutationPending} onClick={() => handleDelete(template)}>{deletingTemplateId === template.id ? 'Deleting…' : 'Delete'}</Button></div> : null}</article>)}
          </div>
          <DefinitionCatalogPagination appliedSearch={templateSearch} disabled={mutationPending} isLoading={isLoadingTemplates} itemCount={templates.length} limitHint="Up to 100 templates may be stored." meta={templateMeta} nextLabel="Next template page" noun="email templates" pageNumber={templatePage} previousLabel="Previous template page" setPageNumber={setTemplatePage} />
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
          <DefinitionCatalogFilters applyLabel="Apply snippet search" disabled={mutationPending} handleSearch={applySnippetSearch} isLoading={isLoadingSnippets} searchInput={snippetSearchInput} searchLabel="Search email snippets" searchPlaceholder="Snippet name" setSearchInput={setSnippetSearchInput} />
          <div className="record-list" role="list" aria-label="Email snippets">
            {!isLoadingSnippets && snippets.length === 0 ? <article className="record-row" role="listitem"><div><p>{snippetSearch ? 'No email snippets match this search.' : 'No email snippets yet.'}</p><p className="field-hint">{snippetSearch ? 'Change the search and try again.' : 'Create short answers, CTAs, or scheduling blocks for reuse.'}</p></div></article> : null}
            {snippets.map((snippet) => <article className="record-row" key={snippet.id} role="listitem"><div><h3>{snippet.name}</h3><p className="field-hint">{snippet.body} · revision {snippet.revision}</p></div>{canManage ? <div><Button className="button-secondary" type="button" disabled={mutationPending} onClick={() => startSnippetEdit(snippet)}>Edit</Button><Button className="button-danger" type="button" disabled={mutationPending} onClick={() => handleSnippetDelete(snippet)}>{deletingSnippetId === snippet.id ? 'Deleting…' : 'Delete'}</Button></div> : null}</article>)}
          </div>
          <DefinitionCatalogPagination appliedSearch={snippetSearch} disabled={mutationPending} isLoading={isLoadingSnippets} itemCount={snippets.length} limitHint="Up to 100 snippets may be stored." meta={snippetMeta} nextLabel="Next snippet page" noun="email snippets" pageNumber={snippetPage} previousLabel="Previous snippet page" setPageNumber={setSnippetPage} />
        </div>
      </Card>

      {canManage ? <Card><form className="auth-form card-stack" aria-label={editingId ? 'Edit email template' : 'Create email template'} onSubmit={handleSubmit}><h2>{editingId ? 'Edit template' : 'New template'}</h2><DefinitionTextField form={form} label="Name" maxLength={120} name="name" required setForm={setForm} /><DefinitionTextField form={form} label="Subject" maxLength={500} name="subject" required setForm={setForm} /><DefinitionTextField form={form} label="Body" maxLength={10000} multiline name="body" required rows={8} setForm={setForm} /><div className="button-row"><Button type="submit" disabled={mutationPending}>{isSaving ? 'Saving…' : editingId ? 'Save changes' : 'Create template'}</Button>{editingId ? <Button className="button-secondary" type="button" disabled={mutationPending} onClick={resetForm}>Cancel</Button> : null}</div></form></Card> : null}

      {canManage ? <Card><form className="auth-form card-stack" aria-label={editingSnippetId ? 'Edit email snippet' : 'Create email snippet'} onSubmit={handleSnippetSubmit}><h2>{editingSnippetId ? 'Edit snippet' : 'New snippet'}</h2><DefinitionTextField form={snippetForm} label="Snippet name" maxLength={120} name="name" required setForm={setSnippetForm} /><DefinitionTextField form={snippetForm} label="Snippet body" maxLength={10000} multiline name="body" rows={5} setForm={setSnippetForm} /><div className="button-row"><Button type="submit" disabled={mutationPending}>{isSavingSnippet ? 'Saving…' : editingSnippetId ? 'Save snippet' : 'Create snippet'}</Button>{editingSnippetId ? <Button className="button-secondary" type="button" disabled={mutationPending} onClick={resetSnippetForm}>Cancel</Button> : null}</div></form></Card> : null}
    </section>
  )
}

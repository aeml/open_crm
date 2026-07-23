import { useRef, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { createLeadLandingPage, listLeadLandingPagePage, publicLeadLandingPageURL, updateLeadLandingPage } from '../lib/landing_pages'
import { usePageTitle } from '../lib/use_page_title'
import { LeadSurfaceCatalogControls, useLeadSurfaceCatalog } from './lead_surface_catalog'

function emptyForm(firstLeadFormId = '') {
  return {
    name: '',
    slug: '',
    title: '',
    subtitle: '',
    body: '',
    ctaLabel: 'Submit',
    theme: 'light',
    leadCaptureFormId: firstLeadFormId,
    isActive: true,
    revision: 0,
    retainedLeadFormName: ''
  }
}

function formFromPage(page) {
  return {
    name: page.name || '',
    slug: page.slug || '',
    title: page.title || page.name || '',
    subtitle: page.subtitle || '',
    body: page.body || '',
    ctaLabel: page.ctaLabel || 'Submit',
    theme: page.theme || 'light',
    leadCaptureFormId: String(page.leadCaptureFormId || ''),
    isActive: page.isActive !== false,
    revision: page.revision,
    retainedLeadFormName: page.leadCaptureFormName || `Lead form ${page.leadCaptureFormId}`
  }
}

function landingPagePayload(form) {
  return {
    name: form.name,
    slug: form.slug,
    title: form.title,
    subtitle: form.subtitle,
    body: form.body,
    ctaLabel: form.ctaLabel,
    theme: form.theme,
    leadCaptureFormId: Number(form.leadCaptureFormId),
	isActive: form.isActive,
	...(form.revision > 0 ? { revision: form.revision } : {})
  }
}

export function SettingsLandingPagesRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Landing Pages')
  const {
    items: pages, meta: pageMeta, leadForms, form, setForm, error, setError,
    isLoading, statusFilter, setStatusFilter, pageNumber, setPageNumber,
    load: loadLandingPages
  } = useLeadSurfaceCatalog({
    listPage: listLeadLandingPagePage,
    itemKey: 'pages',
    emptyForm,
    loadErrorMessage: 'Unable to load landing pages.'
  })
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const operationPending = useRef(false)

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm(leadForms[0]?.id ? String(leadForms[0].id) : ''))
  }

  function startEdit(page) {
    setEditingId(page.id)
    setForm(formFromPage(page))
    setStatus('')
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage || operationPending.current) return

    operationPending.current = true
    setIsSaving(true)
    setStatus('')
    try {
      const payload = landingPagePayload(form)
      if (editingId) {
        await updateLeadLandingPage(editingId, payload)
        setStatus('Landing page updated.')
      } else {
		await createLeadLandingPage(payload)
        setStatus('Landing page created.')
      }
      resetForm()
      setError('')
      if (pageNumber === 1) await loadLandingPages({ requestedPage: 1 })
      else setPageNumber(1)
    } catch (saveError) {
      setError(saveError.message || 'Unable to save landing page.')
    } finally {
      setIsSaving(false)
      operationPending.current = false
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Landing pages</h2>
              <p>Publish simple hosted pages that pair a marketing message with one active lead form.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading landing pages...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadLandingPages()} retryLabel="Retry landing pages" /> : null}
          <LeadSurfaceCatalogControls
            label="Landing page status" itemCount={pages.length} meta={pageMeta} noun="landing pages"
            statusFilter={statusFilter} setStatusFilter={setStatusFilter} pageNumber={pageNumber}
            setPageNumber={setPageNumber} isLoading={isLoading} isSaving={isSaving}
            previousLabel="Previous landing page" nextLabel="Next landing page"
          >
          <div className="record-list" role="list" aria-label="Landing pages">
            {!isLoading && pages.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No landing pages yet.</p>
                  <p className="field-hint">Create a page after at least one lead form exists.</p>
                </div>
              </article>
            ) : pages.map((page) => (
              <article className={page.isActive ? 'record-row' : 'record-row record-row-alert'} key={page.id} role="listitem">
                <div>
                  <h3>{page.name}</h3>
                  <p className="field-hint">/{page.slug} · form {page.leadCaptureFormName || page.leadCaptureFormId} · theme {page.theme}</p>
                  <p><a href={publicLeadLandingPageURL(page.slug)} target="_blank" rel="noreferrer">{publicLeadLandingPageURL(page.slug)}</a></p>
                </div>
                <div>
                  <span className="chip">{page.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(page)}>Edit</Button> : null}
                </div>
              </article>
            ))}
          </div>
          </LeadSurfaceCatalogControls>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit landing page' : 'New landing page'}</h2>
              <p className="field-hint">Foundation pages use one hero section, one body block, and an embedded lead form.</p>
            </div>
            {leadForms.length === 0 ? <p className="form-error" role="alert">Create an active lead form before publishing landing pages.</p> : null}
            <Field label="Lead form">
              <select className="text-input" value={form.leadCaptureFormId} onChange={(event) => setForm({ ...form, leadCaptureFormId: event.target.value })} required>
                <option value="">Choose a lead form</option>
				{form.leadCaptureFormId && !leadForms.some((leadForm) => String(leadForm.id) === String(form.leadCaptureFormId)) ? <option value={form.leadCaptureFormId}>{form.retainedLeadFormName} (inactive; retained)</option> : null}
                {leadForms.map((leadForm) => (
                  <option key={leadForm.id} value={leadForm.id}>{leadForm.name}</option>
                ))}
              </select>
            </Field>
            <Field label="Name">
			  <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Demo campaign page" maxLength="100" required />
            </Field>
            <Field label="Slug" hint="Public URL path, globally unique across hosted pages.">
			  <input className="text-input" value={form.slug} onChange={(event) => setForm({ ...form, slug: event.target.value })} placeholder="demo-request" maxLength="80" />
            </Field>
            <Field label="Title">
			  <input className="text-input" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Book a better CRM demo" maxLength="200" />
            </Field>
            <Field label="Subtitle">
			  <input className="text-input" value={form.subtitle} onChange={(event) => setForm({ ...form, subtitle: event.target.value })} placeholder="A focused page for one offer or campaign." maxLength="500" />
            </Field>
            <Field label="Body">
			  <textarea className="text-input" rows={5} value={form.body} onChange={(event) => setForm({ ...form, body: event.target.value })} placeholder="Explain the offer, audience, and next step." maxLength="10000" />
            </Field>
            <Field label="CTA label">
			  <input className="text-input" value={form.ctaLabel} onChange={(event) => setForm({ ...form, ctaLabel: event.target.value })} maxLength="100" required />
            </Field>
            <Field label="Theme">
              <select className="text-input" value={form.theme} onChange={(event) => setForm({ ...form, theme: event.target.value })}>
                <option value="light">Light</option>
                <option value="blue">Blue</option>
                <option value="dark">Dark</option>
              </select>
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active landing page
            </label>
            <div>
              <Button type="submit" disabled={isSaving || leadForms.length === 0}>{isSaving ? 'Saving...' : editingId ? 'Save landing page' : 'Create landing page'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

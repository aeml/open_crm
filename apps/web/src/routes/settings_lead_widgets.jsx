import { useRef, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { createLeadChatWidget, leadChatWidgetEmbedCode, listLeadChatWidgetPage, publicLeadChatWidgetURL, updateLeadChatWidget } from '../lib/lead_chat_widgets'
import { usePageTitle } from '../lib/use_page_title'
import { LeadSurfaceCatalogControls, useLeadSurfaceCatalog } from './lead_surface_catalog'

function emptyForm(firstLeadFormId = '') {
  return {
    name: '',
    title: 'Need help?',
    welcomeMessage: 'Hi. Tell us a little about what you need and we will follow up.',
    promptLabel: 'Chat with us',
    ctaLabel: 'Send',
    theme: 'light',
    position: 'bottom-right',
    leadCaptureFormId: firstLeadFormId,
	isActive: true,
	revision: 0,
	retainedLeadFormName: ''
  }
}

function formFromWidget(widget) {
  return {
    name: widget.name || '',
    title: widget.title || widget.name || '',
    welcomeMessage: widget.welcomeMessage || '',
    promptLabel: widget.promptLabel || 'Chat with us',
    ctaLabel: widget.ctaLabel || 'Send',
    theme: widget.theme || 'light',
    position: widget.position || 'bottom-right',
    leadCaptureFormId: String(widget.leadCaptureFormId || ''),
	isActive: widget.isActive !== false,
	revision: widget.revision,
	retainedLeadFormName: widget.leadCaptureFormName || `Lead form ${widget.leadCaptureFormId}`
  }
}

function payloadFromForm(form) {
  return {
    name: form.name,
    title: form.title,
    welcomeMessage: form.welcomeMessage,
    promptLabel: form.promptLabel,
    ctaLabel: form.ctaLabel,
    theme: form.theme,
    position: form.position,
    leadCaptureFormId: Number(form.leadCaptureFormId),
	isActive: form.isActive,
	...(form.revision > 0 ? { revision: form.revision } : {})
  }
}

export function SettingsLeadWidgetsRoute() {
  const { canAdminister: canManage } = useAuth()
  usePageTitle('Website Widgets')
  const {
    items: widgets, meta: widgetMeta, leadForms, form, setForm, error, setError,
    isLoading, statusFilter, setStatusFilter, pageNumber, setPageNumber,
    load: loadWidgets
  } = useLeadSurfaceCatalog({
    listPage: listLeadChatWidgetPage,
    itemKey: 'widgets',
    emptyForm,
    loadErrorMessage: 'Unable to load website widgets.'
  })
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const operationPending = useRef(false)

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm(leadForms[0]?.id ? String(leadForms[0].id) : ''))
  }

  function startEdit(widget) {
    setEditingId(widget.id)
    setForm(formFromWidget(widget))
    setStatus('')
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (!canManage || operationPending.current) return

    operationPending.current = true
    setIsSaving(true)
    setStatus('')
    try {
      const payload = payloadFromForm(form)
      if (editingId) {
        await updateLeadChatWidget(editingId, payload)
        setStatus('Website widget updated.')
      } else {
		await createLeadChatWidget(payload)
        setStatus('Website widget created.')
      }
      resetForm()
      setError('')
      if (pageNumber === 1) await loadWidgets({ requestedPage: 1 })
      else setPageNumber(1)
    } catch (saveError) {
      setError(saveError.message || 'Unable to save website widget.')
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
              <h2>Website widgets</h2>
              <p>Embed a compact chat-style lead form on your site without adding live chat yet.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading website widgets...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadWidgets()} retryLabel="Retry widgets" /> : null}
          <LeadSurfaceCatalogControls
            label="Website widget status" itemCount={widgets.length} meta={widgetMeta} noun="website widgets"
            statusFilter={statusFilter} setStatusFilter={setStatusFilter} pageNumber={pageNumber}
            setPageNumber={setPageNumber} isLoading={isLoading} isSaving={isSaving}
            previousLabel="Previous widget page" nextLabel="Next widget page"
          >
          <div className="record-list" role="list" aria-label="Website widgets">
            {!isLoading && widgets.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No website widgets yet.</p>
                  <p className="field-hint">Create a widget after at least one lead form exists.</p>
                </div>
              </article>
            ) : widgets.map((widget) => (
              <article className={widget.isActive ? 'record-row' : 'record-row record-row-alert'} key={widget.id} role="listitem">
                <div>
                  <h3>{widget.name}</h3>
                  <p className="field-hint">{widget.promptLabel} | form {widget.leadCaptureFormName || widget.leadCaptureFormId} | {widget.theme} | {widget.position}</p>
                  <p><a href={publicLeadChatWidgetURL(widget.publicId)} target="_blank" rel="noreferrer">{publicLeadChatWidgetURL(widget.publicId)}</a></p>
                  <textarea className="text-input" rows={4} readOnly value={leadChatWidgetEmbedCode(widget)} aria-label={`${widget.name} embed code`} />
                </div>
                <div>
                  <span className="chip">{widget.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(widget)}>Edit</Button> : null}
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
              <h2>{editingId ? 'Edit website widget' : 'New website widget'}</h2>
              <p className="field-hint">Foundation widgets render an iframe with one prompt and one lead form.</p>
            </div>
            {leadForms.length === 0 ? <p className="form-error" role="alert">Create an active lead form before publishing website widgets.</p> : null}
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
			  <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Website chat" maxLength="100" required />
            </Field>
            <Field label="Title">
			  <input className="text-input" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} placeholder="Need help?" maxLength="200" required />
            </Field>
            <Field label="Welcome message">
			  <textarea className="text-input" rows={3} value={form.welcomeMessage} onChange={(event) => setForm({ ...form, welcomeMessage: event.target.value })} maxLength="2000" />
            </Field>
            <Field label="Prompt label">
			  <input className="text-input" value={form.promptLabel} onChange={(event) => setForm({ ...form, promptLabel: event.target.value })} maxLength="100" required />
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
            <Field label="Position">
              <select className="text-input" value={form.position} onChange={(event) => setForm({ ...form, position: event.target.value })}>
                <option value="bottom-right">Bottom right</option>
                <option value="bottom-left">Bottom left</option>
                <option value="inline">Inline</option>
              </select>
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active website widget
            </label>
            <div>
              <Button type="submit" disabled={isSaving || leadForms.length === 0}>{isSaving ? 'Saving...' : editingId ? 'Save website widget' : 'Create website widget'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

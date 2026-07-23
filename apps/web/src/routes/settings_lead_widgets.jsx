import { Field } from '../components/ui/field'
import { useAuth } from '../app/providers'
import { createLeadChatWidget, leadChatWidgetEmbedCode, listLeadChatWidgetPage, publicLeadChatWidgetURL, updateLeadChatWidget } from '../lib/lead_chat_widgets'
import { usePageTitle } from '../lib/use_page_title'
import { LeadSurfaceCatalogCard, LeadSurfaceCatalogItem, LeadSurfaceEditorCard, LeadSurfaceTextField, LeadSurfaceThemeField, useLeadSurfaceCatalog, useLeadSurfaceEditor } from './lead_surface_catalog'

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
  const { editingId, status, isSaving, resetForm, startEdit, handleSubmit } = useLeadSurfaceEditor({
    canManage, leadForms, form, setForm, setError, pageNumber, setPageNumber,
    load: loadWidgets, emptyForm, formFromItem: formFromWidget,
    payloadFromForm, createItem: createLeadChatWidget, updateItem: updateLeadChatWidget,
    createdMessage: 'Website widget created.', updatedMessage: 'Website widget updated.',
    saveErrorMessage: 'Unable to save website widget.'
  })

  return (
    <section className="dashboard-grid settings-grid">
      <LeadSurfaceCatalogCard
        title="Website widgets"
        description="Embed a compact chat-style lead form on your site without adding live chat yet."
        loadingMessage="Loading website widgets..." status={status} error={error}
        onRetry={() => loadWidgets()} retryLabel="Retry widgets"
        items={widgets} meta={widgetMeta} noun="website widgets" statusLabel="Website widget status"
        statusFilter={statusFilter} setStatusFilter={setStatusFilter}
        pageNumber={pageNumber} setPageNumber={setPageNumber}
        isLoading={isLoading} isSaving={isSaving}
        previousLabel="Previous widget page" nextLabel="Next widget page"
        ariaLabel="Website widgets" emptyMessage="No website widgets yet."
        emptyHint="Create a widget after at least one lead form exists."
        renderItem={(widget) => (
          <LeadSurfaceCatalogItem key={widget.id} item={widget} canManage={canManage} onEdit={startEdit}>
            <h3>{widget.name}</h3>
            <p className="field-hint">{widget.promptLabel} | form {widget.leadCaptureFormName || widget.leadCaptureFormId} | {widget.theme} | {widget.position}</p>
            <p><a href={publicLeadChatWidgetURL(widget.publicId)} target="_blank" rel="noreferrer">{publicLeadChatWidgetURL(widget.publicId)}</a></p>
            <textarea className="text-input" rows={4} readOnly value={leadChatWidgetEmbedCode(widget)} aria-label={`${widget.name} embed code`} />
          </LeadSurfaceCatalogItem>
        )}
      />

      {canManage ? (
        <LeadSurfaceEditorCard
          editingId={editingId} editTitle="Edit website widget" newTitle="New website widget"
          description="Foundation widgets render an iframe with one prompt and one lead form."
          noFormsMessage="Create an active lead form before publishing website widgets."
          leadForms={leadForms} form={form} setForm={setForm} activeLabel="Active website widget"
          isSaving={isSaving} onSubmit={handleSubmit} saveLabel="Save website widget"
          createLabel="Create website widget" onCancel={resetForm}
        >
          <LeadSurfaceTextField label="Name" name="name" form={form} setForm={setForm} placeholder="Website chat" maxLength="100" required />
          <LeadSurfaceTextField label="Title" name="title" form={form} setForm={setForm} placeholder="Need help?" maxLength="200" required />
          <LeadSurfaceTextField label="Welcome message" name="welcomeMessage" form={form} setForm={setForm} multiline rows={3} maxLength="2000" />
          <LeadSurfaceTextField label="Prompt label" name="promptLabel" form={form} setForm={setForm} maxLength="100" required />
          <LeadSurfaceTextField label="CTA label" name="ctaLabel" form={form} setForm={setForm} maxLength="100" required />
          <LeadSurfaceThemeField form={form} setForm={setForm} />
          <Field label="Position">
            <select className="text-input" value={form.position} onChange={(event) => setForm({ ...form, position: event.target.value })}>
              <option value="bottom-right">Bottom right</option>
              <option value="bottom-left">Bottom left</option>
              <option value="inline">Inline</option>
            </select>
          </Field>
        </LeadSurfaceEditorCard>
      ) : null}
    </section>
  )
}

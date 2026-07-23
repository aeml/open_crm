import { useAuth } from '../app/providers'
import { createLeadLandingPage, listLeadLandingPagePage, publicLeadLandingPageURL, updateLeadLandingPage } from '../lib/landing_pages'
import { usePageTitle } from '../lib/use_page_title'
import { LeadSurfaceCatalogCard, LeadSurfaceCatalogItem, LeadSurfaceEditorCard, LeadSurfaceTextField, LeadSurfaceThemeField, useLeadSurfaceCatalog, useLeadSurfaceEditor } from './lead_surface_catalog'

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
  const { editingId, status, isSaving, resetForm, startEdit, handleSubmit } = useLeadSurfaceEditor({
    canManage, leadForms, form, setForm, setError, pageNumber, setPageNumber,
    load: loadLandingPages, emptyForm, formFromItem: formFromPage,
    payloadFromForm: landingPagePayload, createItem: createLeadLandingPage,
    updateItem: updateLeadLandingPage, createdMessage: 'Landing page created.',
    updatedMessage: 'Landing page updated.', saveErrorMessage: 'Unable to save landing page.'
  })

  return (
    <section className="dashboard-grid settings-grid">
      <LeadSurfaceCatalogCard
        title="Landing pages"
        description="Publish simple hosted pages that pair a marketing message with one active lead form."
        loadingMessage="Loading landing pages..." status={status} error={error}
        onRetry={() => loadLandingPages()} retryLabel="Retry landing pages"
        items={pages} meta={pageMeta} noun="landing pages" statusLabel="Landing page status"
        statusFilter={statusFilter} setStatusFilter={setStatusFilter}
        pageNumber={pageNumber} setPageNumber={setPageNumber}
        isLoading={isLoading} isSaving={isSaving}
        previousLabel="Previous landing page" nextLabel="Next landing page"
        ariaLabel="Landing pages" emptyMessage="No landing pages yet."
        emptyHint="Create a page after at least one lead form exists."
        renderItem={(page) => (
          <LeadSurfaceCatalogItem key={page.id} item={page} canManage={canManage} onEdit={startEdit}>
            <h3>{page.name}</h3>
            <p className="field-hint">/{page.slug} · form {page.leadCaptureFormName || page.leadCaptureFormId} · theme {page.theme}</p>
            <p><a href={publicLeadLandingPageURL(page.slug)} target="_blank" rel="noreferrer">{publicLeadLandingPageURL(page.slug)}</a></p>
          </LeadSurfaceCatalogItem>
        )}
      />

      {canManage ? (
        <LeadSurfaceEditorCard
          editingId={editingId} editTitle="Edit landing page" newTitle="New landing page"
          description="Foundation pages use one hero section, one body block, and an embedded lead form."
          noFormsMessage="Create an active lead form before publishing landing pages."
          leadForms={leadForms} form={form} setForm={setForm} activeLabel="Active landing page"
          isSaving={isSaving} onSubmit={handleSubmit} saveLabel="Save landing page"
          createLabel="Create landing page" onCancel={resetForm}
        >
          <LeadSurfaceTextField label="Name" name="name" form={form} setForm={setForm} placeholder="Demo campaign page" maxLength="100" required />
          <LeadSurfaceTextField label="Slug" hint="Public URL path, globally unique across hosted pages." name="slug" form={form} setForm={setForm} placeholder="demo-request" maxLength="80" />
          <LeadSurfaceTextField label="Title" name="title" form={form} setForm={setForm} placeholder="Book a better CRM demo" maxLength="200" />
          <LeadSurfaceTextField label="Subtitle" name="subtitle" form={form} setForm={setForm} placeholder="A focused page for one offer or campaign." maxLength="500" />
          <LeadSurfaceTextField label="Body" name="body" form={form} setForm={setForm} multiline rows={5} placeholder="Explain the offer, audience, and next step." maxLength="10000" />
          <LeadSurfaceTextField label="CTA label" name="ctaLabel" form={form} setForm={setForm} maxLength="100" required />
          <LeadSurfaceThemeField form={form} setForm={setForm} />
        </LeadSurfaceEditorCard>
      ) : null}
    </section>
  )
}

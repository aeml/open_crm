import { useEffect, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listLeadCaptureForms } from '../lib/lead_forms'

export const leadSurfacePageSize = 50

export function useLeadSurfaceCatalog({ listPage, itemKey, emptyForm, loadErrorMessage }) {
  const [items, setItems] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: leadSurfacePageSize, total: 0 })
  const [leadForms, setLeadForms] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [statusFilter, setStatusFilter] = useState('all')
  const [pageNumber, setPageNumber] = useState(1)
  const latestLoad = useRef(0)
  const dependenciesLoaded = useRef(false)

  async function load({ signal, requestedPage = pageNumber, surfaceStatus = statusFilter, refreshDependencies = false } = {}) {
    const loadID = latestLoad.current + 1
    latestLoad.current = loadID
    setIsLoading(true)
    try {
      const loadDependencies = refreshDependencies || !dependenciesLoaded.current
      const [catalog, nextForms] = await Promise.all([
        listPage({ status: surfaceStatus, page: requestedPage, pageSize: leadSurfacePageSize, signal }),
        loadDependencies ? listLeadCaptureForms({ status: 'active', signal }) : Promise.resolve(null)
      ])
      if (signal?.aborted || loadID !== latestLoad.current) return null
      setItems(catalog[itemKey])
      setMeta(catalog.meta)
      if (loadDependencies) {
        setLeadForms(nextForms)
        setForm((current) => current.leadCaptureFormId
          ? current
          : { ...current, leadCaptureFormId: nextForms[0]?.id ? String(nextForms[0].id) : '' })
        dependenciesLoaded.current = true
      }
      setError('')
      return catalog
    } catch (loadError) {
      if (!isAbortError(loadError) && loadID === latestLoad.current) {
        setError(loadError.message || loadErrorMessage)
      }
      return null
    } finally {
      if (!signal?.aborted && loadID === latestLoad.current) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [pageNumber, statusFilter])

  return {
    items, meta, leadForms, form, setForm, error, setError, isLoading,
    statusFilter, setStatusFilter, pageNumber, setPageNumber, load
  }
}

export function useLeadSurfaceEditor({
  canManage, leadForms, form, setForm, setError, pageNumber, setPageNumber,
  load, emptyForm, formFromItem, payloadFromForm, createItem, updateItem,
  createdMessage, updatedMessage, saveErrorMessage
}) {
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const operationPending = useRef(false)

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm(leadForms[0]?.id ? String(leadForms[0].id) : ''))
  }

  function startEdit(item) {
    setEditingId(item.id)
    setForm(formFromItem(item))
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
      if (editingId !== null) {
        await updateItem(editingId, payload)
        setStatus(updatedMessage)
      } else {
        await createItem(payload)
        setStatus(createdMessage)
      }
      resetForm()
      setError('')
      if (pageNumber === 1) await load({ requestedPage: 1 })
      else setPageNumber(1)
    } catch (saveError) {
      setError(saveError.message || saveErrorMessage)
    } finally {
      setIsSaving(false)
      operationPending.current = false
    }
  }

  return { editingId, status, isSaving, resetForm, startEdit, handleSubmit }
}

export function LeadSurfaceCatalogControls({
  label, itemCount, meta, noun, statusFilter, setStatusFilter,
  pageNumber, setPageNumber, isLoading, isSaving, previousLabel, nextLabel, children
}) {
  return (
    <>
      <Field label={label}>
        <select className="text-input" value={statusFilter} disabled={isLoading || isSaving} onChange={(event) => { setPageNumber(1); setStatusFilter(event.target.value) }}>
          <option value="all">Active and inactive</option>
          <option value="active">Active</option>
          <option value="inactive">Inactive</option>
        </select>
      </Field>
      {children}
      <p className="field-hint" role="status">Showing {itemCount} of {meta.total} {noun}.</p>
      <div className="button-row">
        <Button className="button-secondary" type="button" disabled={isLoading || isSaving || pageNumber <= 1} onClick={() => setPageNumber((current) => current - 1)}>{previousLabel}</Button>
        <Button className="button-secondary" type="button" disabled={isLoading || isSaving || pageNumber * meta.pageSize >= meta.total} onClick={() => setPageNumber((current) => current + 1)}>{nextLabel}</Button>
      </div>
    </>
  )
}

export function LeadSurfaceCatalogCard({
  title, description, loadingMessage, status, error, onRetry, retryLabel,
  items, meta, noun, statusLabel, statusFilter, setStatusFilter, pageNumber,
  setPageNumber, isLoading, isSaving, previousLabel, nextLabel, ariaLabel,
  emptyMessage, emptyHint, renderItem
}) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header"><div><h2>{title}</h2><p>{description}</p></div></div>
        {isLoading ? <p className="field-hint">{loadingMessage}</p> : null}
        {status ? <p className="field-hint" role="status">{status}</p> : null}
        {error ? <InlineError message={error} onRetry={onRetry} retryLabel={retryLabel} /> : null}
        <LeadSurfaceCatalogControls
          label={statusLabel} itemCount={items.length} meta={meta} noun={noun}
          statusFilter={statusFilter} setStatusFilter={setStatusFilter}
          pageNumber={pageNumber} setPageNumber={setPageNumber}
          isLoading={isLoading} isSaving={isSaving}
          previousLabel={previousLabel} nextLabel={nextLabel}
        >
          <div className="record-list" role="list" aria-label={ariaLabel}>
            {!isLoading && items.length === 0 ? (
              <article className="record-row" role="listitem">
                <div><p>{emptyMessage}</p><p className="field-hint">{emptyHint}</p></div>
              </article>
            ) : items.map(renderItem)}
          </div>
        </LeadSurfaceCatalogControls>
      </div>
    </Card>
  )
}

export function LeadSurfaceCatalogItem({ item, canManage, onEdit, children }) {
  return (
    <article className={item.isActive ? 'record-row' : 'record-row record-row-alert'} role="listitem">
      <div>{children}</div>
      <div>
        <span className="chip">{item.isActive ? 'Active' : 'Inactive'}</span>
        {canManage ? <Button className="button-secondary" type="button" onClick={() => onEdit(item)}>Edit</Button> : null}
      </div>
    </article>
  )
}

export function LeadSurfaceEditorCard({
  editingId, editTitle, newTitle, description, noFormsMessage, leadForms,
  form, setForm, activeLabel, isSaving, onSubmit, saveLabel, createLabel,
  onCancel, children
}) {
  const editing = editingId !== null
  return (
    <Card>
      <form className="auth-form card-stack" onSubmit={onSubmit}>
        <div><h2>{editing ? editTitle : newTitle}</h2><p className="field-hint">{description}</p></div>
        {leadForms.length === 0 ? <p className="form-error" role="alert">{noFormsMessage}</p> : null}
        <Field label="Lead form">
          <select className="text-input" value={form.leadCaptureFormId} onChange={(event) => setForm((current) => ({ ...current, leadCaptureFormId: event.target.value }))} required>
            <option value="">Choose a lead form</option>
            {form.leadCaptureFormId && !leadForms.some((leadForm) => String(leadForm.id) === String(form.leadCaptureFormId)) ? <option value={form.leadCaptureFormId}>{form.retainedLeadFormName} (inactive; retained)</option> : null}
            {leadForms.map((leadForm) => <option key={leadForm.id} value={leadForm.id}>{leadForm.name}</option>)}
          </select>
        </Field>
        {children}
        <label className="field-hint">
          <input type="checkbox" checked={form.isActive} onChange={(event) => setForm((current) => ({ ...current, isActive: event.target.checked }))} /> {activeLabel}
        </label>
        <div>
          <Button type="submit" disabled={isSaving || leadForms.length === 0}>{isSaving ? 'Saving...' : editing ? saveLabel : createLabel}</Button>
          {editing ? <Button className="button-secondary" type="button" onClick={onCancel}>Cancel</Button> : null}
        </div>
      </form>
    </Card>
  )
}

export function LeadSurfaceTextField({ label, hint, name, form, setForm, multiline = false, ...props }) {
  const controlProps = {
    ...props,
    className: 'text-input',
    value: form[name],
    onChange: (event) => setForm((current) => ({ ...current, [name]: event.target.value }))
  }
  return (
    <Field label={label} hint={hint}>
      {multiline ? <textarea {...controlProps} /> : <input {...controlProps} />}
    </Field>
  )
}

export function LeadSurfaceThemeField({ form, setForm }) {
  return (
    <Field label="Theme">
      <select className="text-input" value={form.theme} onChange={(event) => setForm((current) => ({ ...current, theme: event.target.value }))}>
        <option value="light">Light</option>
        <option value="blue">Blue</option>
        <option value="dark">Dark</option>
      </select>
    </Field>
  )
}

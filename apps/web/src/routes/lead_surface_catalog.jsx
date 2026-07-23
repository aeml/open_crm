import { useEffect, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
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

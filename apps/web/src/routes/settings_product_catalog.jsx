import { useEffect, useRef, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { archiveProductCatalogItem, createProductCatalogItem, listProductCatalogPage, updateProductCatalogItem } from '../lib/product_catalog'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = {
  name: '',
  sku: '',
  description: '',
  itemType: 'product',
  unitPrice: '0.00',
  currency: 'USD',
  unitName: 'unit',
  isActive: true
}
const pageSize = 50
const emptyMeta = { page: 1, pageSize, total: 0 }

function formFromItem(item) {
  return {
    name: item.name || '',
    sku: item.sku || '',
    description: item.description || '',
    itemType: item.itemType || 'product',
    unitPrice: item.unitPrice || '0.00',
    currency: item.currency || 'USD',
    unitName: item.unitName || 'unit',
    isActive: item.isActive !== false
  }
}

function catalogPayload(form) {
  return {
    name: form.name,
    sku: form.sku,
    description: form.description,
    itemType: form.itemType,
    unitPrice: form.unitPrice,
    currency: form.currency,
    unitName: form.unitName,
    isActive: form.isActive
  }
}

function priceLabel(item) {
  const unit = item.unitName || 'unit'
  return `${item.currency || 'USD'} ${item.unitPrice || '0.00'} / ${unit}`
}

export function SettingsProductCatalogRoute() {
  const { session, canWrite: canManage } = useAuth()
  usePageTitle('Product Catalog')
  const [items, setItems] = useState([])
  const [meta, setMeta] = useState(emptyMeta)
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)
  const [pendingArchiveId, setPendingArchiveId] = useState(null)
  const [searchInput, setSearchInput] = useState('')
  const [appliedSearch, setAppliedSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState('all')
  const [pageNumber, setPageNumber] = useState(1)
  const operationPending = useRef(false)
  const latestLoad = useRef(0)

  async function loadCatalog({ signal, requestedPage = pageNumber, search = appliedSearch, itemStatus = statusFilter } = {}) {
    const loadID = latestLoad.current + 1
    latestLoad.current = loadID
    setIsLoading(true)
    try {
      const next = await listProductCatalogPage({ search, status: itemStatus, page: requestedPage, pageSize, signal })
      if (signal?.aborted || loadID !== latestLoad.current) return null
      setItems(next.items)
      setMeta(next.meta)
      setError('')
      return next
    } catch (loadError) {
      if (!isAbortError(loadError) && loadID === latestLoad.current) {
        setError(loadError.message || 'Unable to load product catalog.')
      }
    } finally {
      if (!signal?.aborted && loadID === latestLoad.current) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadCatalog({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [appliedSearch, pageNumber, statusFilter])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm)
  }

  function startEdit(item) {
    setEditingId(item.id)
    setForm(formFromItem(item))
    setStatus('')
  }

  async function handleSubmit(event) {
    event.preventDefault()
    if (operationPending.current) return
    operationPending.current = true
    setIsSaving(true)
    setStatus('')
    try {
      const payload = catalogPayload(form)
      if (editingId) {
        await updateProductCatalogItem(editingId, payload)
        setStatus('Catalog item updated.')
      } else {
        await createProductCatalogItem(payload)
        setStatus('Catalog item created.')
      }
      resetForm()
      setError('')
      if (pageNumber === 1) await loadCatalog()
      else setPageNumber(1)
    } catch (saveError) {
      setError(saveError.message || 'Unable to save catalog item.')
    } finally {
      setIsSaving(false)
      operationPending.current = false
    }
  }

  async function handleArchive(itemId) {
    if (operationPending.current) return
    operationPending.current = true
    setPendingArchiveId(itemId)
    setStatus('')
    try {
      await archiveProductCatalogItem(itemId)
      if (editingId === itemId) {
        resetForm()
      }
      setStatus('Catalog item archived.')
      setError('')
      const next = await loadCatalog()
      if (next && next.items.length === 0 && next.meta.total > 0 && pageNumber > 1) setPageNumber(pageNumber - 1)
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive catalog item.')
    } finally {
      setPendingArchiveId(null)
      operationPending.current = false
    }
  }

  function handleSearch(event) {
    event.preventDefault()
    const nextSearch = searchInput.trim()
    if (nextSearch === appliedSearch && pageNumber === 1) loadCatalog({ requestedPage: 1, search: nextSearch })
    else {
      setPageNumber(1)
      setAppliedSearch(nextSearch)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Product catalog</h2>
              <p>Maintain reusable products and services for {session?.organization?.name || 'your team'} before quotes and line items are added.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading product catalog...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadCatalog()} retryLabel="Retry product catalog" /> : null}
          <form className="filters-grid" onSubmit={handleSearch}>
            <Field label="Search product catalog">
              <input className="text-input" maxLength={100} value={searchInput} disabled={isSaving || pendingArchiveId !== null} onChange={(event) => setSearchInput(event.target.value)} placeholder="Name or SKU" />
            </Field>
            <Field label="Catalog status">
              <select className="text-input" value={statusFilter} disabled={isSaving || pendingArchiveId !== null} onChange={(event) => { setPageNumber(1); setStatusFilter(event.target.value) }}>
                <option value="all">Active and inactive</option>
                <option value="active">Active</option>
                <option value="inactive">Inactive</option>
              </select>
            </Field>
            <Button className="button-secondary" type="submit" disabled={isLoading || isSaving || pendingArchiveId !== null}>Apply search</Button>
          </form>
          <div className="record-list" role="list" aria-label="Product catalog items">
            {!isLoading && items.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>{appliedSearch || statusFilter !== 'all' ? 'No catalog items match these filters.' : 'No catalog items yet.'}</p>
                  <p className="field-hint">{appliedSearch || statusFilter !== 'all' ? 'Change the search or status filter and try again.' : 'Create services, products, SKUs, and prices your team sells repeatedly.'}</p>
                </div>
              </article>
            ) : items.map((item) => (
              <article className={item.isActive ? 'record-row' : 'record-row record-row-alert'} key={item.id} role="listitem">
                <div>
                  <h3>{item.name}</h3>
                  <p className="field-hint">{item.itemType === 'service' ? 'Service' : 'Product'} · {item.sku || 'No SKU'} · {priceLabel(item)}</p>
                  {item.description ? <p className="field-hint">{item.description}</p> : null}
                </div>
                <div>
                  <span className="chip">{item.isActive ? 'Active' : 'Inactive'}</span>
                  {canManage ? <Button className="button-secondary" type="button" disabled={isSaving || pendingArchiveId !== null} onClick={() => startEdit(item)}>Edit</Button> : null}
                  {canManage && item.isActive ? <Button className="button-secondary" type="button" disabled={isSaving || pendingArchiveId !== null} onClick={() => handleArchive(item.id)}>{pendingArchiveId === item.id ? 'Archiving...' : 'Archive'}</Button> : null}
                </div>
              </article>
            ))}
          </div>
          <p className="field-hint" role="status">Showing {items.length} of {meta.total} catalog items{appliedSearch ? ` matching “${appliedSearch}”` : ''}. Up to 100 items may be active for quote selection.</p>
          <div>
            <Button className="button-secondary" type="button" disabled={isLoading || pageNumber <= 1 || isSaving || pendingArchiveId !== null} onClick={() => setPageNumber((current) => current - 1)}>Previous page</Button>
            <Button className="button-secondary" type="button" disabled={isLoading || pageNumber * meta.pageSize >= meta.total || isSaving || pendingArchiveId !== null} onClick={() => setPageNumber((current) => current + 1)}>Next page</Button>
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit catalog item' : 'New catalog item'}</h2>
              <p className="field-hint">Prices are stored as fixed two-decimal amounts with a three-letter currency.</p>
            </div>
            <Field label="Name">
              <input className="text-input" maxLength={150} value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Implementation package" required />
            </Field>
            <Field label="SKU">
              <input className="text-input" maxLength={80} value={form.sku} onChange={(event) => setForm({ ...form, sku: event.target.value })} placeholder="SERV-001" />
            </Field>
            <Field label="Type">
              <select className="text-input" value={form.itemType} onChange={(event) => setForm({ ...form, itemType: event.target.value })}>
                <option value="product">Product</option>
                <option value="service">Service</option>
              </select>
            </Field>
            <Field label="Unit price">
              <input className="text-input" inputMode="decimal" maxLength={13} pattern="\d{1,10}(\.\d{1,2})?" value={form.unitPrice} onChange={(event) => setForm({ ...form, unitPrice: event.target.value })} placeholder="150.00" required />
            </Field>
            <Field label="Currency">
              <input className="text-input" maxLength={3} value={form.currency} onChange={(event) => setForm({ ...form, currency: event.target.value.toUpperCase() })} required />
            </Field>
            <Field label="Unit">
              <input className="text-input" maxLength={50} value={form.unitName} onChange={(event) => setForm({ ...form, unitName: event.target.value })} placeholder="hour" required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" maxLength={2000} rows={4} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active catalog item
            </label>
            <div>
              <Button type="submit" disabled={isSaving || pendingArchiveId !== null}>{isSaving ? 'Saving...' : editingId ? 'Save catalog item' : 'Create catalog item'}</Button>
              {editingId ? <Button className="button-secondary" type="button" disabled={isSaving || pendingArchiveId !== null} onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

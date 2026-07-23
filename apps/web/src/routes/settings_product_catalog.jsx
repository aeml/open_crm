import { useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { archiveProductCatalogItem, createProductCatalogItem, listProductCatalogPage, updateProductCatalogItem } from '../lib/product_catalog'
import { usePageTitle } from '../lib/use_page_title'
import { DefinitionCatalogFilters, DefinitionCatalogPagination, DefinitionTextField, useDefinitionCatalog } from './definition_catalog'

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
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [isSaving, setIsSaving] = useState(false)
  const [pendingArchiveId, setPendingArchiveId] = useState(null)
  const {
    appliedSearch, error, handleSearch, isLoading, items, load: loadCatalog,
    meta, operationPending, pageNumber, searchInput, setError, setPageNumber,
    setSearchInput, setStatusFilter, statusFilter
  } = useDefinitionCatalog({
    requestPage: listProductCatalogPage,
    loadErrorMessage: 'Unable to load product catalog.'
  })

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

  const mutationPending = isSaving || pendingArchiveId !== null

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
          <DefinitionCatalogFilters
            disabled={mutationPending} handleSearch={handleSearch} isLoading={isLoading}
            searchInput={searchInput} searchLabel="Search product catalog" searchPlaceholder="Name or SKU"
            setPageNumber={setPageNumber} setSearchInput={setSearchInput} setStatusFilter={setStatusFilter}
            statusFilter={statusFilter} statusLabel="Catalog status"
          >
            <option value="all">Active and inactive</option>
            <option value="active">Active</option>
            <option value="inactive">Inactive</option>
          </DefinitionCatalogFilters>
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
                  {canManage ? <Button className="button-secondary" type="button" disabled={mutationPending} onClick={() => startEdit(item)}>Edit</Button> : null}
                  {canManage && item.isActive ? <Button className="button-secondary" type="button" disabled={mutationPending} onClick={() => handleArchive(item.id)}>{pendingArchiveId === item.id ? 'Archiving...' : 'Archive'}</Button> : null}
                </div>
              </article>
            ))}
          </div>
          <DefinitionCatalogPagination
            appliedSearch={appliedSearch} className="" disabled={mutationPending} isLoading={isLoading}
            itemCount={items.length} limitHint="Up to 100 items may be active for quote selection."
            meta={meta} noun="catalog items" pageNumber={pageNumber} setPageNumber={setPageNumber}
          />
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit catalog item' : 'New catalog item'}</h2>
              <p className="field-hint">Prices are stored as fixed two-decimal amounts with a three-letter currency.</p>
            </div>
            <DefinitionTextField form={form} label="Name" maxLength={150} name="name" placeholder="Implementation package" required setForm={setForm} />
            <DefinitionTextField form={form} label="SKU" maxLength={80} name="sku" placeholder="SERV-001" setForm={setForm} />
            <Field label="Type">
              <select className="text-input" value={form.itemType} onChange={(event) => setForm({ ...form, itemType: event.target.value })}>
                <option value="product">Product</option>
                <option value="service">Service</option>
              </select>
            </Field>
            <DefinitionTextField form={form} inputMode="decimal" label="Unit price" maxLength={13} name="unitPrice" pattern="\d{1,10}(\.\d{1,2})?" placeholder="150.00" required setForm={setForm} />
            <Field label="Currency">
              <input className="text-input" maxLength={3} value={form.currency} onChange={(event) => setForm({ ...form, currency: event.target.value.toUpperCase() })} required />
            </Field>
            <DefinitionTextField form={form} label="Unit" maxLength={50} name="unitName" placeholder="hour" required setForm={setForm} />
            <DefinitionTextField form={form} label="Description" maxLength={2000} multiline name="description" rows={4} setForm={setForm} />
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active catalog item
            </label>
            <div>
              <Button type="submit" disabled={mutationPending}>{isSaving ? 'Saving...' : editingId ? 'Save catalog item' : 'Create catalog item'}</Button>
              {editingId ? <Button className="button-secondary" type="button" disabled={mutationPending} onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

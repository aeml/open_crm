import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { archiveProductCatalogItem, createProductCatalogItem, listProductCatalogItems, updateProductCatalogItem } from '../lib/product_catalog'
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
  const { session } = useAuth()
  usePageTitle('Product Catalog')
  const role = session?.membership?.role || ''
  const canManage = role !== 'viewer'
  const [items, setItems] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadCatalog({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextItems = await listProductCatalogItems({ signal })
      setItems(nextItems)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load product catalog.')
      }
    } finally {
      if (!signal?.aborted) {
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
  }, [])

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
    setIsSaving(true)
    setStatus('')
    try {
      const payload = catalogPayload(form)
      if (editingId) {
        const updated = await updateProductCatalogItem(editingId, payload)
        setItems((current) => current.map((item) => (item.id === editingId ? updated : item)))
        setStatus('Catalog item updated.')
      } else {
        const created = await createProductCatalogItem(payload)
        setItems((current) => [...current, created])
        setStatus('Catalog item created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save catalog item.')
    } finally {
      setIsSaving(false)
    }
  }

  async function handleArchive(itemId) {
    setStatus('')
    try {
      await archiveProductCatalogItem(itemId)
      setItems((current) => current.map((item) => (item.id === itemId ? { ...item, isActive: false } : item)))
      if (editingId === itemId) {
        resetForm()
      }
      setStatus('Catalog item archived.')
      setError('')
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive catalog item.')
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
          <div className="record-list" role="list" aria-label="Product catalog items">
            {!isLoading && items.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No catalog items yet.</p>
                  <p className="field-hint">Create services, products, SKUs, and prices your team sells repeatedly.</p>
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
                  {canManage ? <Button className="button-secondary" type="button" onClick={() => startEdit(item)}>Edit</Button> : null}
                  {canManage && item.isActive ? <Button className="button-secondary" type="button" onClick={() => handleArchive(item.id)}>Archive</Button> : null}
                </div>
              </article>
            ))}
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
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Implementation package" required />
            </Field>
            <Field label="SKU">
              <input className="text-input" value={form.sku} onChange={(event) => setForm({ ...form, sku: event.target.value })} placeholder="SERV-001" />
            </Field>
            <Field label="Type">
              <select className="text-input" value={form.itemType} onChange={(event) => setForm({ ...form, itemType: event.target.value })}>
                <option value="product">Product</option>
                <option value="service">Service</option>
              </select>
            </Field>
            <Field label="Unit price">
              <input className="text-input" inputMode="decimal" pattern="\d+(\.\d{1,2})?" value={form.unitPrice} onChange={(event) => setForm({ ...form, unitPrice: event.target.value })} placeholder="150.00" required />
            </Field>
            <Field label="Currency">
              <input className="text-input" maxLength={3} value={form.currency} onChange={(event) => setForm({ ...form, currency: event.target.value.toUpperCase() })} required />
            </Field>
            <Field label="Unit">
              <input className="text-input" value={form.unitName} onChange={(event) => setForm({ ...form, unitName: event.target.value })} placeholder="hour" required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={4} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} />
            </Field>
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active catalog item
            </label>
            <div>
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save catalog item' : 'Create catalog item'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

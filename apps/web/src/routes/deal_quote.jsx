import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { formatMoney, formatSignatureTime, signatureStatusLabel } from './deal_view'

export function DealLineItemsCard({
  canWrite,
  deal,
  form,
  isSaving,
  items,
  labels,
  onAdd,
  onCatalogChange,
  onRemove,
  onSave,
  onSetForm,
  products,
  totals
}) {
  const currency = totals.currency || deal.valueCurrency
  const setField = (name) => (event) => onSetForm((current) => ({ ...current, [name]: event.target.value }))
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Line items</h3>
            <p className="field-hint">Use catalog items or custom entries. Saving line items updates the {labels.singular.toLowerCase()} value.</p>
          </div>
          <div>
            <p>{formatMoney(totals.total, currency)}</p>
            <p className="field-hint">Subtotal {formatMoney(totals.subtotal, currency)} · Discount {formatMoney(totals.discountTotal, currency)} · Tax {formatMoney(totals.taxTotal, currency)}</p>
          </div>
        </div>
        <div className="record-list" role="list" aria-label="Deal line items">
          {items.length === 0 ? (
            <article className="record-row" role="listitem">
              <div>
                <p>No line items yet.</p>
                <p className="field-hint">Add products or services to calculate the deal value from quote-ready details.</p>
              </div>
            </article>
          ) : items.map((item, index) => (
            <article className="record-row" key={`${item.id || 'draft'}-${index}`} role="listitem">
              <div>
                <h4>{item.name}</h4>
                <p className="field-hint">{item.sku || 'No SKU'} · {item.quantity} {item.unitName || 'unit'} x {formatMoney(item.unitPrice, item.currency)} · Discount {formatMoney(item.discountAmount, item.currency)} · Tax {item.taxRate || '0'}%</p>
              </div>
              <div>
                <p>{item.total ? formatMoney(item.total, item.currency) : 'Unsaved'}</p>
                {canWrite ? <Button className="button-secondary" type="button" onClick={() => onRemove(index)}>Remove</Button> : null}
              </div>
            </article>
          ))}
        </div>
        {canWrite ? (
          <>
            <form className="auth-form" onSubmit={onAdd}>
              <Field label="Catalog item">
                <select className="text-input" value={form.productCatalogItemId} onChange={onCatalogChange}>
                  <option value="">Custom line item</option>
                  {products.map((item) => <option key={item.id} value={item.id}>{item.sku ? `${item.name} (${item.sku})` : item.name}</option>)}
                </select>
              </Field>
              <Field label="Line item name">
                <input className="text-input" value={form.name} onChange={setField('name')} required />
              </Field>
              <Field label="Line item type">
                <select className="text-input" value={form.itemType} onChange={setField('itemType')}>
                  <option value="product">Product</option>
                  <option value="service">Service</option>
                </select>
              </Field>
              <Field label="Line item quantity">
                <input className="text-input" inputMode="decimal" value={form.quantity} onChange={setField('quantity')} required />
              </Field>
              <Field label="Line item unit">
                <input className="text-input" value={form.unitName} onChange={setField('unitName')} required />
              </Field>
              <Field label="Line item unit price">
                <input className="text-input" inputMode="decimal" value={form.unitPrice} onChange={setField('unitPrice')} required />
              </Field>
              <Field label="Line item discount">
                <input className="text-input" inputMode="decimal" value={form.discountAmount} onChange={setField('discountAmount')} />
              </Field>
              <Field label="Line item tax rate">
                <input className="text-input" inputMode="decimal" value={form.taxRate} onChange={setField('taxRate')} />
              </Field>
              <Field label="Line item currency">
                <input className="text-input" maxLength={3} value={form.currency} onChange={(event) => onSetForm((current) => ({ ...current, currency: event.target.value.toUpperCase() }))} required />
              </Field>
              <Button type="submit">Add line item</Button>
            </form>
            <Button type="button" onClick={onSave} disabled={isSaving}>{isSaving ? 'Saving...' : 'Save line items'}</Button>
          </>
        ) : null}
      </div>
    </Card>
  )
}

export function DealSignatureCard({ canWrite, form, isCreating, isSnapshotPending, onCreate, onSetForm, onUpdate, requests, updatingID }) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Proposal tracking</h3>
            <p className="field-hint">Manual CRM tracking only. Open CRM does not send this proposal or collect a legal e-signature yet.</p>
          </div>
        </div>
        <div className="record-list" role="list" aria-label="Deal proposal tracking records">
          {requests.length === 0 ? (
            <article className="record-row" role="listitem">
              <div>
                <p>No tracked proposals yet.</p>
                <p className="field-hint">Create a tracking record after you deliver a proposal outside Open CRM.</p>
              </div>
            </article>
          ) : requests.map((request) => (
            <article className="record-row" key={request.id} role="listitem">
              <div>
                <h4>{request.signerName}</h4>
                <p className="field-hint">{request.signerEmail} · {signatureStatusLabel(request.status)} · {request.provider || 'native_tracking'}</p>
                <p className="field-hint">Filename reference: {request.quoteFileName || 'Current quote PDF'} · PDF content remains live</p>
              </div>
              <div>
                <p>{request.status === 'signed' ? `Signed ${formatSignatureTime(request.signedAt)}` : `Updated ${formatSignatureTime(request.updatedAt)}`}</p>
                {canWrite ? (
                  <select className="text-input" aria-label={`Proposal status for ${request.signerName}`} value={request.status} disabled={isSnapshotPending || updatingID === request.id} onChange={(event) => onUpdate(request.id, event.target.value)}>
                    <option value="draft">Draft</option>
                    <option value="sent">Sent</option>
                    <option value="signed">Signed</option>
                    <option value="declined">Declined</option>
                    <option value="voided">Voided</option>
                  </select>
                ) : null}
              </div>
            </article>
          ))}
        </div>
        {canWrite ? (
          <form className="auth-form" onSubmit={onCreate}>
            <Field label="Recipient name">
              <input className="text-input" value={form.signerName} onChange={(event) => onSetForm((current) => ({ ...current, signerName: event.target.value }))} required />
            </Field>
            <Field label="Recipient email">
              <input className="text-input" type="email" value={form.signerEmail} onChange={(event) => onSetForm((current) => ({ ...current, signerEmail: event.target.value }))} required />
            </Field>
            <Button type="submit" disabled={isSnapshotPending}>{isCreating ? 'Creating...' : 'Create proposal tracking'}</Button>
          </form>
        ) : null}
      </div>
    </Card>
  )
}

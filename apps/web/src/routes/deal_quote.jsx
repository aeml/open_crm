import { useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { signatureCertificateURL } from '../lib/deals'
import { CloseReviewFields, emptyCloseReview } from './deal_close_review'
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

function SignedQuoteConversion({ deal, isConverting, isSnapshotPending, onConvert, request, stages }) {
  const [form, setForm] = useState({ stageId: '', ...emptyCloseReview })
  const wonStages = stages.filter((stage) => stage.isClosed && stage.isWon)
  if (request.convertedAt) {
    return (
      <div className="inline-note" aria-label={`Signed quote conversion for ${request.quoteNumber}`}>
        <strong>Converted to {request.conversionStageName}</strong>
        <p className="field-hint">{request.conversionCloseReasonLabel}{request.conversionCloseNotes ? ` · ${request.conversionCloseNotes}` : ''}</p>
        <p className="field-hint">Converted {formatSignatureTime(request.convertedAt)}{request.convertedByUserName ? ` by ${request.convertedByUserName}` : ''}. Later stage changes do not erase this retained conversion evidence.</p>
      </div>
    )
  }
  if (deal.status !== 'open') {
    return <p className="field-hint">This signed quote was not used to close the deal. Conversion is available only while the deal is open.</p>
  }
  return (
    <details className="quote-delivery-composer">
      <summary>Convert signed quote to won</summary>
      <form className="auth-form" aria-label={`Convert ${request.quoteNumber} to won`} onSubmit={(event) => { event.preventDefault(); onConvert(request.id, form) }}>
        <p className="field-hint">This deliberate action atomically links the certificate to the won outcome, close review, stage history, automation, and customer handoff.</p>
        <Field label={`Won stage for ${request.quoteNumber}`}>
          <select className="text-input" value={form.stageId} onChange={(event) => setForm((current) => ({ ...current, stageId: event.target.value }))} required>
            <option value="">Choose a won stage</option>
            {wonStages.map((stage) => <option key={stage.id} value={stage.id}>{stage.name}</option>)}
          </select>
        </Field>
        <CloseReviewFields outcome="won" value={form} onChange={setForm} />
        {wonStages.length === 0 ? <p className="form-error" role="alert">Configure a won stage before converting this signed quote.</p> : null}
        <Button type="submit" disabled={isSnapshotPending || isConverting || wonStages.length === 0 || !form.stageId || !form.closeReasonCode}>{isConverting ? 'Converting…' : 'Convert signed quote and hand off client'}</Button>
      </form>
    </details>
  )
}

export function DealSignatureCard({ canWrite, convertingID, deal, dealID, isSnapshotPending, onConvert, onVoid, requests, stages, voidingID }) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Quote signatures</h3>
            <p className="field-hint">Signature requests are bound to immutable quote bytes and a recipient-specific email link. Staff can void an unsigned request but cannot mark it signed.</p>
          </div>
        </div>
        <div className="record-list" role="list" aria-label="Deal quote signature requests">
          {requests.length === 0 ? (
            <article className="record-row" role="listitem">
              <div>
                <p>No quote signature requests yet.</p>
                <p className="field-hint">Finalize a quote, then choose “Request electronic signature” when delivering that version.</p>
              </div>
            </article>
          ) : requests.map((request) => (
            <article className="record-row" key={request.id} role="listitem">
              <div>
                <h4>{request.quoteNumber || 'Historical proposal record'} · {request.signerName}</h4>
                <p className="field-hint">{request.signerEmail} · {signatureStatusLabel(request.status)}{request.signingExpired && request.status === 'sent' ? ' · Signing deadline passed' : ''}</p>
                {request.provider === 'open_crm_native' ? (
                  <p className="field-hint">Immutable file: {request.quoteFileName} · Authentication: recipient-specific email link</p>
                ) : (
                  <p className="field-hint">Historical manual tracking only. This record is not evidence of a signature collected by Open CRM.</p>
                )}
                {request.status === 'signed' ? <p className="field-hint">Typed signature: {request.signedName} · consent recorded {formatSignatureTime(request.consentedAt)}</p> : null}
                {request.status === 'declined' && request.declinedReason ? <p className="field-hint">Recipient reason: {request.declinedReason}</p> : null}
                {canWrite && request.provider === 'open_crm_native' && request.status === 'signed' ? (
                  <SignedQuoteConversion
                    deal={deal}
                    isConverting={convertingID === request.id}
                    isSnapshotPending={isSnapshotPending}
                    onConvert={onConvert}
                    request={request}
                    stages={stages}
                  />
                ) : null}
              </div>
              <div>
                <p>{request.status === 'signed' ? `Signed ${formatSignatureTime(request.signedAt)}` : `Updated ${formatSignatureTime(request.updatedAt)}`}</p>
                {request.status === 'signed' ? <a className="button button-secondary" href={signatureCertificateURL(dealID, request.id)}>Download certificate</a> : null}
                {canWrite && request.provider === 'open_crm_native' && request.status === 'sent' ? (
                  <Button className="button-danger" type="button" disabled={isSnapshotPending || voidingID === request.id} onClick={() => onVoid(request.id)}>{voidingID === request.id ? 'Voiding…' : 'Void unsigned request'}</Button>
                ) : null}
              </div>
            </article>
          ))}
        </div>
      </div>
    </Card>
  )
}

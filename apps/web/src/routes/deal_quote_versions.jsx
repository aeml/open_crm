import { useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { quoteVersionPDFURL } from '../lib/deals'
import { formatMoney, formatSignatureTime, signatureStatusLabel } from './deal_view'

const deliveryStatusLabels = { sent: 'Sent', uncertain: 'Needs resolution', failed: 'Not sent', sending: 'Sending' }
const quoteLifecycleLabels = { expired: 'Expired', superseded: 'Replaced' }

function DeliveryEvidence({ delivery, isResolving, onResolve, signature }) {
  return (
    <article className={delivery.status === 'uncertain' ? 'inline-note quote-delivery-alert' : 'inline-note'}>
      <div className="section-header">
        <div>
          <strong>{deliveryStatusLabels[delivery.status] || 'Prepared'}</strong>
          <p className="field-hint">{delivery.senderEmail} → {delivery.recipientEmail}</p>
        </div>
        {delivery.sentAt ? <span className="chip">{formatSignatureTime(delivery.sentAt)}</span> : null}
      </div>
      {delivery.status === 'sent' ? (
        <div className="quote-delivery-evidence">
          <span>Link accesses: {delivery.accessCount || 0}</span>
          <span>PDF downloads: {delivery.downloadCount || 0}</span>
          <span>{delivery.receiptConfirmedAt ? `Receipt confirmed ${formatSignatureTime(delivery.receiptConfirmedAt)}` : 'Receipt not confirmed'}</span>
        </div>
      ) : null}
      {delivery.signatureRequestId ? <p className="field-hint">Signature: {signature ? signatureStatusLabel(signature.status) : 'Requested'}</p> : null}
      {delivery.lastError ? <p className="field-hint">{delivery.lastError}</p> : null}
      {delivery.status === 'uncertain' ? (
        <div className="button-row" aria-label={`Resolve delivery ${delivery.id}`}>
          <Button className="button-secondary" disabled={isResolving} onClick={() => onResolve(delivery.id, 'confirmed_sent')}>Confirm sent</Button>
          <Button className="button-secondary" disabled={isResolving} onClick={() => onResolve(delivery.id, 'retry')}>Retry</Button>
          <Button className="button-danger" disabled={isResolving} onClick={() => onResolve(delivery.id, 'not_sent')}>Mark not sent</Button>
        </div>
      ) : null}
    </article>
  )
}

function QuoteVersionRow({ canWrite, deal, isDelivering, isSnapshotPending, onDeliver, onReissue, onResolve, quote, resolvingDeliveryId, signatureRequests, validUntil }) {
  const [deliveryForm, setDeliveryForm] = useState(() => ({
    subject: `Finalized quote ${quote.quoteNumber}`,
    messageBody: `Hi ${quote.recipientName},\n\nPlease review ${quote.quoteNumber}.`,
    requestSignature: false
  }))
  const deliveries = quote.deliveries || []
  const unresolved = deliveries.some((delivery) => ['prepared', 'sending', 'uncertain'].includes(delivery.status))
  const lifecycleStatus = quote.lifecycleStatus || 'active'
  const signed = signatureRequests.some((request) => request.quoteId === quote.id && request.status === 'signed')
  const setField = (name) => (event) => setDeliveryForm((current) => ({ ...current, [name]: event.target.value }))

  return (
    <article className="record-row quote-version-row" role="listitem">
      <div className="quote-version-summary">
        <div>
          <div className="button-row">
            <h4>{quote.quoteNumber}</h4>
            <span className="chip">{quoteLifecycleLabels[lifecycleStatus] || 'Active'}</span>
          </div>
          <p className="field-hint">
            Version {quote.version} · {quote.recipientName} &lt;{quote.recipientEmail}&gt; · valid through {quote.validUntil}
          </p>
          <p className="field-hint">
            Finalized {formatSignatureTime(quote.createdAt)} by {quote.createdByUserName} · SHA-256 {quote.pdfSha256}
          </p>
          {quote.reissuedFromQuoteNumber || quote.reissuedByQuoteNumber ? <p className="field-hint">{quote.reissuedFromQuoteNumber ? `Reissued from immutable ${quote.reissuedFromQuoteNumber}.` : `Replaced by ${quote.reissuedByQuoteNumber}; use it for delivery.`}</p> : null}
        </div>
        <div>
          <p>{formatMoney(quote.total, quote.currency)}</p>
          <a className="button button-secondary" href={quoteVersionPDFURL(deal.id, quote.id)}>
            Download {quote.quoteNumber}
          </a>
        </div>
      </div>
      {deliveries.length > 0 ? (
        <div className="quote-delivery-list" aria-label={`Deliveries for ${quote.quoteNumber}`}>
          {deliveries.map((delivery) => (
            <DeliveryEvidence
              delivery={delivery}
              isResolving={resolvingDeliveryId === delivery.id}
              key={delivery.id}
              onResolve={(deliveryID, resolution) => onResolve(quote.id, deliveryID, resolution)}
              signature={signatureRequests.find((request) => request.id === delivery.signatureRequestId)}
            />
          ))}
          <p className="field-hint">Scanners may open links; confirmation is not acceptance.</p>
        </div>
      ) : null}
      {canWrite && lifecycleStatus === 'expired' ? (
        <div className="inline-note">
          <strong>Expired quotes cannot be delivered.</strong>
          {unresolved ? <p className="form-error" role="alert">Resolve this delivery before reissuing.</p> : null}
          {signed ? <p className="field-hint">Signed evidence requires a revised quote.</p> : deal.status !== 'open' ? <p className="field-hint">Reopen the deal before reissuing.</p> : (
            <><p className="field-hint">Copies the snapshot using the date below; original evidence is unchanged.</p><Button disabled={isSnapshotPending || unresolved} onClick={() => onReissue(quote, validUntil)}>Create replacement quote</Button></>
          )}
        </div>
      ) : null}
      {canWrite && lifecycleStatus === 'active' ? (
        <details className="quote-delivery-composer">
          <summary>{deliveries.length > 0 ? 'Deliver again' : 'Deliver by email'}</summary>
          <form className="auth-form" aria-label={`Deliver ${quote.quoteNumber}`} onSubmit={(event) => { event.preventDefault(); onDeliver(quote, deliveryForm) }}>
            <Field label={`Email subject for ${quote.quoteNumber}`}>
              <input className="text-input" maxLength={500} value={deliveryForm.subject} onChange={setField('subject')} required />
            </Field>
            <Field label={`Message for ${quote.quoteNumber}`}>
              <textarea className="text-input" maxLength={10000} rows="4" value={deliveryForm.messageBody} onChange={setField('messageBody')} required />
            </Field>
            <label className="checkbox-row">
              <input type="checkbox" checked={deliveryForm.requestSignature} onChange={(event) => setDeliveryForm((current) => ({ ...current, requestSignature: event.target.checked }))} />
              Request signature from {quote.recipientName}
            </label>
            <p className="field-hint">Connected mailbox; expiring link. {deliveryForm.requestSignature ? 'Consent creates an audit certificate.' : 'Review does not collect acceptance.'} No automatic retry.</p>
            {unresolved ? <p className="form-error" role="alert">Resolve the current delivery before creating another send.</p> : null}
            <Button type="submit" disabled={isDelivering || unresolved}>{isDelivering ? 'Delivering…' : deliveryForm.requestSignature ? 'Send for signature' : 'Send quote'}</Button>
          </form>
        </details>
      ) : null}
    </article>
  )
}

export function DealQuoteVersionsCard({
  areLineItemsDirty,
  canWrite,
  deal,
  deliveringQuoteId,
  form,
  isFinalizing,
  isSnapshotPending,
  lineItems,
  onDeliver,
  onFinalize,
  onReissue,
  onResolveDelivery,
  onSetForm,
  quotes,
  resolvingDeliveryId,
  signatureRequests
}) {
  const setField = (name) => (event) => onSetForm((current) => ({ ...current, [name]: event.target.value }))
  const canFinalize = lineItems.length > 0 && !areLineItemsDirty

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Finalized quotes</h3>
            <p className="field-hint">Immutable PDF, digest, and commercial snapshot.</p>
          </div>
        </div>
        <div className="record-list" role="list" aria-label="Finalized deal quotes">
          {quotes.length === 0 ? (
            <article className="record-row" role="listitem">
              <div>
                <p>No finalized quote versions yet.</p>
                <p className="field-hint">Save a line item, then finalize a quote.</p>
              </div>
            </article>
          ) : quotes.map((quote) => (
            <QuoteVersionRow
              canWrite={canWrite}
              deal={deal}
              isDelivering={deliveringQuoteId === quote.id}
              isSnapshotPending={isSnapshotPending}
              key={quote.id}
              onDeliver={onDeliver}
              onReissue={onReissue}
              onResolve={onResolveDelivery}
              quote={quote}
              resolvingDeliveryId={resolvingDeliveryId}
              signatureRequests={signatureRequests}
              validUntil={form.validUntil}
            />
          ))}
        </div>
        {canWrite ? (
          <form className="auth-form" aria-label="Finalize quote form" onSubmit={onFinalize}>
            <Field label="Recipient name">
              <input className="text-input" maxLength={200} value={form.recipientName} onChange={setField('recipientName')} required />
            </Field>
            <Field label="Recipient email">
              <input className="text-input" type="email" maxLength={320} value={form.recipientEmail} onChange={setField('recipientEmail')} required />
            </Field>
            <Field label="Valid until">
              <input className="text-input" type="date" value={form.validUntil} onChange={setField('validUntil')} required />
            </Field>
            <Field label="Terms">
              <textarea className="text-input" maxLength={10000} rows="4" value={form.terms} onChange={setField('terms')} required />
            </Field>
            {!canFinalize ? <p className="field-hint">Save line-item changes before finalizing.</p> : null}
            <Button type="submit" disabled={!canFinalize || isSnapshotPending}>
              {isFinalizing ? 'Finalizing...' : 'Finalize quote'}
            </Button>
          </form>
        ) : null}
      </div>
    </Card>
  )
}

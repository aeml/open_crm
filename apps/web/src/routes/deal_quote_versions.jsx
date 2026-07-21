import { useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { quoteVersionPDFURL } from '../lib/deals'
import { formatMoney, formatSignatureTime } from './deal_view'

function deliveryStatusLabel(status) {
  if (status === 'sent') return 'Sent'
  if (status === 'uncertain') return 'Needs resolution'
  if (status === 'failed') return 'Not sent'
  if (status === 'sending') return 'Sending'
  return 'Prepared'
}

function DeliveryEvidence({ delivery, isResolving, onResolve }) {
  const accessed = delivery.accessCount || 0
  const downloaded = delivery.downloadCount || 0
  return (
    <article className={delivery.status === 'uncertain' ? 'inline-note quote-delivery-alert' : 'inline-note'}>
      <div className="section-header">
        <div>
          <strong>{deliveryStatusLabel(delivery.status)}</strong>
          <p className="field-hint">{delivery.senderEmail} → {delivery.recipientEmail}</p>
        </div>
        {delivery.sentAt ? <span className="chip">{formatSignatureTime(delivery.sentAt)}</span> : null}
      </div>
      {delivery.status === 'sent' ? (
        <div className="quote-delivery-evidence">
          <span>Link accesses: {accessed}</span>
          <span>PDF downloads: {downloaded}</span>
          <span>{delivery.receiptConfirmedAt ? `Receipt confirmed ${formatSignatureTime(delivery.receiptConfirmedAt)}` : 'Receipt not confirmed'}</span>
        </div>
      ) : null}
      {delivery.lastError ? <p className="field-hint">{delivery.lastError}</p> : null}
      {delivery.status === 'uncertain' ? (
        <div className="button-row" aria-label={`Resolve delivery ${delivery.id}`}>
          <Button className="button-secondary" disabled={isResolving} onClick={() => onResolve(delivery.id, 'confirmed_sent')}>Confirm in Sent folder</Button>
          <Button className="button-secondary" disabled={isResolving} onClick={() => onResolve(delivery.id, 'retry')}>Retry after checking</Button>
          <Button className="button-danger" disabled={isResolving} onClick={() => onResolve(delivery.id, 'not_sent')}>Mark not sent</Button>
        </div>
      ) : null}
    </article>
  )
}

function QuoteVersionRow({ canWrite, dealID, isDelivering, onDeliver, onResolve, quote, resolvingDeliveryId }) {
  const [deliveryForm, setDeliveryForm] = useState(() => ({
    subject: `Finalized quote ${quote.quoteNumber}`,
    messageBody: `Hi ${quote.recipientName},\n\nPlease review the attached finalized quote for ${quote.total} ${quote.currency}.`
  }))
  const deliveries = quote.deliveries || []
  const unresolved = deliveries.some((delivery) => ['prepared', 'sending', 'uncertain'].includes(delivery.status))
  const setField = (name) => (event) => setDeliveryForm((current) => ({ ...current, [name]: event.target.value }))

  return (
    <article className="record-row quote-version-row" role="listitem">
      <div className="quote-version-summary">
        <div>
          <h4>{quote.quoteNumber}</h4>
          <p className="field-hint">
            Version {quote.version} · {quote.recipientName} &lt;{quote.recipientEmail}&gt; · valid through {quote.validUntil}
          </p>
          <p className="field-hint">
            Finalized {formatSignatureTime(quote.createdAt)} by {quote.createdByUserName} · SHA-256 {quote.pdfSha256}
          </p>
        </div>
        <div>
          <p>{formatMoney(quote.total, quote.currency)}</p>
          <a className="button button-secondary" href={quoteVersionPDFURL(dealID, quote.id)}>
            Download {quote.quoteNumber}
          </a>
        </div>
      </div>
      {deliveries.length > 0 ? (
        <div className="quote-delivery-list" aria-label={`Delivery history for ${quote.quoteNumber}`}>
          {deliveries.map((delivery) => (
            <DeliveryEvidence
              delivery={delivery}
              isResolving={resolvingDeliveryId === delivery.id}
              key={delivery.id}
              onResolve={(deliveryID, resolution) => onResolve(quote.id, deliveryID, resolution)}
            />
          ))}
          <p className="field-hint">Link access can include email security scanners. Only “receipt confirmed” is an explicit recipient action, and it is not a signature or acceptance.</p>
        </div>
      ) : null}
      {canWrite ? (
        <details className="quote-delivery-composer">
          <summary>{deliveries.length > 0 ? 'Deliver this version again' : 'Deliver this version by email'}</summary>
          <form className="auth-form" aria-label={`Deliver ${quote.quoteNumber}`} onSubmit={(event) => { event.preventDefault(); onDeliver(quote, deliveryForm) }}>
            <Field label={`Delivery subject for ${quote.quoteNumber}`}>
              <input className="text-input" maxLength={500} value={deliveryForm.subject} onChange={setField('subject')} required />
            </Field>
            <Field label={`Delivery message for ${quote.quoteNumber}`}>
              <textarea className="text-input" maxLength={10000} rows="4" value={deliveryForm.messageBody} onChange={setField('messageBody')} required />
            </Field>
            <p className="field-hint">Sent through your connected mailbox with an expiring customer link. Open CRM stores the intent before contacting the provider and will not automatically repeat an uncertain send.</p>
            {unresolved ? <p className="form-error" role="alert">Resolve the current delivery before creating another send.</p> : null}
            <Button type="submit" disabled={isDelivering || unresolved}>{isDelivering ? 'Delivering…' : 'Deliver finalized quote'}</Button>
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
  onResolveDelivery,
  onSetForm,
  quotes,
  resolvingDeliveryId
}) {
  const setField = (name) => (event) => onSetForm((current) => ({ ...current, [name]: event.target.value }))
  const canFinalize = lineItems.length > 0 && !areLineItemsDirty

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>Finalized quotes</h3>
            <p className="field-hint">Each version preserves its recipient, terms, totals, line items, exact PDF bytes, and SHA-256 digest. Finalized versions cannot be edited.</p>
          </div>
        </div>
        <div className="record-list" role="list" aria-label="Finalized deal quotes">
          {quotes.length === 0 ? (
            <article className="record-row" role="listitem">
              <div>
                <p>No finalized quote versions yet.</p>
                <p className="field-hint">Save at least one line item, then finalize a recipient-ready version.</p>
              </div>
            </article>
          ) : quotes.map((quote) => (
            <QuoteVersionRow
              canWrite={canWrite}
              dealID={deal.id}
              isDelivering={deliveringQuoteId === quote.id}
              key={quote.id}
              onDeliver={onDeliver}
              onResolve={onResolveDelivery}
              quote={quote}
              resolvingDeliveryId={resolvingDeliveryId}
            />
          ))}
        </div>
        {canWrite ? (
          <form className="auth-form" aria-label="Finalize quote form" onSubmit={onFinalize}>
            <Field label="Quote recipient name">
              <input className="text-input" maxLength={200} value={form.recipientName} onChange={setField('recipientName')} required />
            </Field>
            <Field label="Quote recipient email">
              <input className="text-input" type="email" maxLength={320} value={form.recipientEmail} onChange={setField('recipientEmail')} required />
            </Field>
            <Field label="Quote valid until">
              <input className="text-input" type="date" value={form.validUntil} onChange={setField('validUntil')} required />
            </Field>
            <Field label="Quote terms">
              <textarea className="text-input" maxLength={10000} rows="4" value={form.terms} onChange={setField('terms')} required />
            </Field>
            {!canFinalize ? <p className="field-hint">Save the current line-item changes before finalizing.</p> : null}
            <Button type="submit" disabled={!canFinalize || isSnapshotPending}>
              {isFinalizing ? 'Finalizing...' : 'Finalize quote version'}
            </Button>
          </form>
        ) : null}
      </div>
    </Card>
  )
}

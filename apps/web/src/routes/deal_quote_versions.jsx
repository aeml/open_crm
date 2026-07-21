import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { quoteVersionPDFURL } from '../lib/deals'
import { formatMoney, formatSignatureTime } from './deal_view'

function QuoteVersionRow({ dealID, quote }) {
  return (
    <article className="record-row" role="listitem">
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
    </article>
  )
}

export function DealQuoteVersionsCard({
  areLineItemsDirty,
  canWrite,
  deal,
  form,
  isFinalizing,
  isSnapshotPending,
  lineItems,
  onFinalize,
  onSetForm,
  quotes
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
          ) : quotes.map((quote) => <QuoteVersionRow dealID={deal.id} key={quote.id} quote={quote} />)}
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

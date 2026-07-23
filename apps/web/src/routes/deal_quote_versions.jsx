import { useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { ControlledTextField, Field } from '../components/ui/field'
import { quoteVersionPDFURL } from '../lib/deals'
import { formatMoney, formatSignatureTime, quoteCurrencyDisclosure, signatureStatusLabel } from './deal_view'

const deliveryStatusLabels = { sent: 'Sent', uncertain: 'Needs resolution', failed: 'Not sent', sending: 'Sending' }
const quoteLifecycleLabels = { expired: 'Expired', superseded: 'Replaced' }
const approvalStatusLabels = { not_required: 'Not required', pending: 'Pending independent review', approved: 'Approved', rejected: 'Rejected' }

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

function QuoteApprovalEvidence({ canAdminister, currentUserId, isDeciding, onDecide, quote }) {
  const [note, setNote] = useState('')
  const approval = quote.approval || { required: false, status: 'not_required' }
  const status = approval.status || (approval.required ? 'pending' : 'not_required')
  const canDecide = canAdminister && status === 'pending' &&
    String(approval.requestedByUserId || '') !== String(currentUserId) &&
    String(quote.createdByUserId || '') !== String(currentUserId)

  return (
    <div className="inline-note" aria-label={`Approval evidence for ${quote.quoteNumber}`}>
      <strong>Approval: {approvalStatusLabels[status] || status}</strong>
      {approval.requestedAt ? <p className="field-hint">Requested {formatSignatureTime(approval.requestedAt)} by {approval.requestedByUserName} for this exact PDF digest.</p> : null}
      {approval.decidedAt ? <p className="field-hint">Decided {formatSignatureTime(approval.decidedAt)} by {approval.decidedByUserName}.</p> : null}
      {approval.decisionNote ? <p className="field-hint">Decision note: {approval.decisionNote}</p> : null}
      {status === 'pending' && !canDecide ? <p className="field-hint">A different active owner or admin must review this immutable version before delivery.</p> : null}
      {canDecide ? (
        <div className="card-stack">
          <Field label={`Review note for ${quote.quoteNumber}`}>
            <textarea className="text-input" maxLength={1000} rows="3" value={note} onChange={(event) => setNote(event.target.value)} />
          </Field>
          <div className="button-row">
            <Button type="button" disabled={isDeciding} onClick={() => onDecide(quote, 'approved', note)}>{isDeciding ? 'Recording…' : 'Approve exact PDF'}</Button>
            <Button className="button-danger" type="button" disabled={isDeciding || !note.trim()} onClick={() => onDecide(quote, 'rejected', note)}>Reject with note</Button>
          </div>
        </div>
      ) : null}
    </div>
  )
}

function QuoteVersionRow({ canAdminister, canWrite, currentUserId, deal, isDeciding, isDelivering, isSnapshotPending, onDecideApproval, onDeliver, onReissue, onResolve, quote, resolvingDeliveryId, signatureRequests, validUntil }) {
  const deliveryDefaults = quote.deliveryDefaults || {}
  const [deliveryForm, setDeliveryForm] = useState(() => ({
    subject: deliveryDefaults.subject || `Finalized quote ${quote.quoteNumber}`,
    messageBody: deliveryDefaults.messageBody || `Hi ${quote.recipientName},\n\nPlease review ${quote.quoteNumber}.`,
    requestSignature: Boolean(deliveryDefaults.requestSignature)
  }))
  const deliveries = quote.deliveries || []
  const unresolved = deliveries.some((delivery) => ['prepared', 'sending', 'uncertain'].includes(delivery.status))
  const lifecycleStatus = quote.lifecycleStatus || 'active'
  const approvalStatus = quote.approval?.status || 'not_required'
  const approvalBlocksDelivery = approvalStatus === 'pending' || approvalStatus === 'rejected'
  const signed = signatureRequests.some((request) => request.quoteId === quote.id && request.status === 'signed')

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
          <p className="field-hint">{quote.template ? `${quote.template.name} · retained revision ${quote.template.revision}` : 'Custom terms · no reusable template linked'}</p>
          {quote.reissuedFromQuoteNumber || quote.reissuedByQuoteNumber ? <p className="field-hint">{quote.reissuedFromQuoteNumber ? `Reissued from immutable ${quote.reissuedFromQuoteNumber}.` : `Replaced by ${quote.reissuedByQuoteNumber}; use it for delivery.`}</p> : null}
          <p className="field-hint">{quoteCurrencyDisclosure(quote)}</p>
        </div>
        <div>
          <p>{formatMoney(quote.total, quote.currency)}</p>
          <a className="button button-secondary" href={quoteVersionPDFURL(deal.id, quote.id)}>
            Download {quote.quoteNumber}
          </a>
        </div>
      </div>
      <QuoteApprovalEvidence
        canAdminister={canAdminister}
        currentUserId={currentUserId}
        isDeciding={isDeciding}
        onDecide={onDecideApproval}
        quote={quote}
      />
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
      {canWrite && lifecycleStatus === 'active' && approvalBlocksDelivery ? (
        <div className="inline-note quote-delivery-alert" role="status">
          <strong>{approvalStatus === 'rejected' ? 'Delivery blocked: approval was rejected.' : 'Delivery blocked until independent approval.'}</strong>
          <p className="field-hint">The immutable PDF and decision evidence remain available above. Create a revised quote after a rejection.</p>
        </div>
      ) : null}
      {canWrite && lifecycleStatus === 'active' && !approvalBlocksDelivery ? (
        <details className="quote-delivery-composer">
          <summary>{deliveries.length > 0 ? 'Deliver again' : 'Deliver by email'}</summary>
          <form className="auth-form" aria-label={`Deliver ${quote.quoteNumber}`} onSubmit={(event) => { event.preventDefault(); onDeliver(quote, deliveryForm) }}>
            <ControlledTextField form={deliveryForm} label={`Email subject for ${quote.quoteNumber}`} maxLength={500} name="subject" required setForm={setDeliveryForm} />
            <ControlledTextField form={deliveryForm} label={`Message for ${quote.quoteNumber}`} maxLength={10000} multiline name="messageBody" required rows="4" setForm={setDeliveryForm} />
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
  canAdminister,
  canWrite,
  currentUserId,
  deal,
  decidingQuoteId,
  deliveringQuoteId,
  form,
  isFinalizing,
  isSnapshotPending,
  lineItems,
  onDeliver,
  onDecideApproval,
  onFinalize,
  onQuoteTemplateChange,
  onReissue,
  onResolveDelivery,
  onSetForm,
  quotes,
  quoteApprovalPolicy,
  quoteTemplates,
  resolvingDeliveryId,
  signatureRequests
}) {
  const canFinalize = lineItems.length > 0 && !areLineItemsDirty
  const availableTemplates = quoteTemplates || []
  const approvalPolicy = quoteApprovalPolicy || { approvalRequired: false, activeApprovers: 0 }
  const selectedTemplate = availableTemplates.find((template) => String(template.id) === String(form.templateId))
  const approvalForced = Boolean(approvalPolicy.approvalRequired || selectedTemplate?.requiresApproval)

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
              canAdminister={canAdminister}
              canWrite={canWrite}
              currentUserId={currentUserId}
              deal={deal}
              isDeciding={decidingQuoteId === quote.id}
              isDelivering={deliveringQuoteId === quote.id}
              isSnapshotPending={isSnapshotPending}
              key={quote.id}
              onDeliver={onDeliver}
              onDecideApproval={onDecideApproval}
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
            <Field label="Quote template">
              <select className="text-input" value={form.templateId || ''} onChange={onQuoteTemplateChange}>
                <option value="">Custom terms</option>
                {availableTemplates.filter((template) => template.isActive).map((template) => (
                  <option key={template.id} value={template.id}>{template.name} (revision {template.revision})</option>
                ))}
              </select>
            </Field>
            <ControlledTextField form={form} label="Recipient name" maxLength={200} name="recipientName" required setForm={onSetForm} />
            <ControlledTextField form={form} label="Recipient email" maxLength={320} name="recipientEmail" required setForm={onSetForm} type="email" />
            <ControlledTextField form={form} label="Valid until" name="validUntil" required setForm={onSetForm} type="date" />
            <ControlledTextField form={form} label="Terms" maxLength={10000} multiline name="terms" readOnly={Boolean(selectedTemplate)} required rows="4" setForm={onSetForm} />
            {selectedTemplate ? <p className="field-hint">Terms are locked to {selectedTemplate.name} revision {selectedTemplate.revision}. Choose Custom terms to edit them.</p> : null}
            <label className="checkbox-row">
              <input
                type="checkbox"
                checked={approvalForced || Boolean(form.requestApproval)}
                disabled={approvalForced}
                onChange={(event) => onSetForm((current) => ({ ...current, requestApproval: event.target.checked }))}
              />
              Require independent owner/admin approval before delivery
            </label>
            {approvalPolicy.approvalRequired ? <p className="field-hint">Workspace policy requires approval. {approvalPolicy.activeApprovers} active owner/admin reviewer(s) are currently available.</p> : selectedTemplate?.requiresApproval ? <p className="field-hint">This template requires approval.</p> : null}
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

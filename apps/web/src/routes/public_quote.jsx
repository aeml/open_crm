import { useEffect, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { confirmPublicDealQuoteReceipt, getPublicDealQuote, publicQuotePDFURL, publicSignatureCertificateURL, updatePublicDealQuote } from '../lib/deals'
import { createIdempotencyKey } from '../lib/idempotency'
import { usePageTitle } from '../lib/use_page_title'
import { formatMoney, formatSignatureTime } from './deal_view'

export function PublicQuoteRoute() {
  const [params] = useSearchParams()
  const token = params.get('token')?.trim() || ''
  const [quote, setQuote] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isConfirming, setIsConfirming] = useState(false)
  const [signatureAction, setSignatureAction] = useState('')
  const signatureAttempts = useRef({})
  usePageTitle(quote?.quoteNumber || 'Finalized Quote')

  async function loadQuote({ signal } = {}) {
    if (!token) {
      setError('This quote link is incomplete. Ask the sender for a new link.')
      setIsLoading(false)
      return
    }
    setIsLoading(true)
    try {
      setQuote(await getPublicDealQuote(token, { signal }))
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load quote.')
    } finally {
      if (!signal?.aborted) setIsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadQuote({ signal: controller.signal })
    return () => controller.abort()
  }, [token])

  async function confirmReceipt() {
    setIsConfirming(true)
    try {
      setQuote(await confirmPublicDealQuoteReceipt(token))
      setError('')
    } catch (confirmationError) {
      setError(confirmationError.message || 'Unable to confirm quote receipt.')
    } finally {
      setIsConfirming(false)
    }
  }

  async function updateSignature(event, action) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const input = action === 'signature'
      ? { signerName: String(form.get('signerName') || '').trim(), consent: form.has('consent') }
      : { reason: String(form.get('reason') || '').trim() }
    const fingerprint = JSON.stringify(input)
    if (signatureAttempts.current[action]?.fingerprint !== fingerprint) {
      signatureAttempts.current[action] = { fingerprint, key: createIdempotencyKey(`quote-${action}`) }
    }
    setSignatureAction(action)
    try {
      setQuote(await updatePublicDealQuote(token, action, input, signatureAttempts.current[action].key))
      setError('')
      delete signatureAttempts.current[action]
    } catch (actionError) {
      setError(actionError.message || `Unable to ${action === 'signature' ? 'sign' : 'decline'} quote.`)
    } finally {
      setSignatureAction('')
    }
  }

  return (
    <main className="landing-page landing-page-light public-quote-page">
      <section className="landing-page-panel public-quote-panel">
        {isLoading ? <p className="field-hint" role="status">Loading finalized quote…</p> : null}
        {error && !quote ? <InlineError message={error} onRetry={() => loadQuote()} retryLabel="Retry quote" /> : null}
        {quote ? (
          <Card>
            <div className="card-stack">
              <header className="public-quote-header">
                <div>
                  <p className="eyebrow">Finalized quote from {quote.organizationName}</p>
                  <h1>{quote.quoteNumber}</h1>
                  <p className="field-hint">Prepared for {quote.recipientName} · sent {formatSignatureTime(quote.sentAt)}</p>
                </div>
                <strong className="public-quote-total">{formatMoney(quote.total, quote.currency)}</strong>
              </header>
              <dl className="public-quote-facts">
                <div><dt>Project</dt><dd>{quote.dealName}</dd></div>
                <div><dt>Valid through</dt><dd>{quote.validUntil}</dd></div>
                <div><dt>PDF SHA-256</dt><dd><code>{quote.pdfSha256}</code></dd></div>
              </dl>
              <section>
                <h2>Terms</h2>
                <p className="message-body public-quote-terms">{quote.terms}</p>
              </section>
              <div className="button-row">
                <a className="button button-primary" href={publicQuotePDFURL(token)}>Download finalized PDF</a>
                {quote.receiptConfirmedAt ? (
                  <span className="inline-note" role="status">Receipt confirmed {formatSignatureTime(quote.receiptConfirmedAt)}</span>
                ) : (
                  <Button disabled={isConfirming} onClick={confirmReceipt}>{isConfirming ? 'Confirming…' : 'Confirm receipt'}</Button>
                )}
              </div>
              {quote.signature ? (
                <section className="public-quote-signature" aria-labelledby="quote-signature-heading">
                  <h2 id="quote-signature-heading">Electronic signature</h2>
                  {quote.signature.status === 'signed' ? (
                    <div className="inline-note" role="status">
                      <strong>Signed by {quote.signature.signedName}</strong>
                      <p>Signed {formatSignatureTime(quote.signature.signedAt)} through the recipient-specific email link.</p>
                      <p>Certificate SHA-256: <code>{quote.signature.certificateSha256}</code></p>
                      <a className="button button-secondary" href={publicSignatureCertificateURL(token)}>Download signature certificate</a>
                    </div>
                  ) : null}
                  {quote.signature.status === 'declined' ? <p className="inline-note" role="status"><strong>Quote declined.</strong> The response was recorded {formatSignatureTime(quote.signature.declinedAt)}.</p> : null}
                  {quote.signature.status === 'voided' ? <p className="inline-note" role="status"><strong>Signature request voided.</strong> Contact the sender if you still want to proceed.</p> : null}
                  {quote.signature.status === 'sent' && !quote.signature.canSign ? <p className="inline-note" role="status"><strong>Signing deadline passed.</strong> Contact the sender for a new finalized quote.</p> : null}
                  {quote.signature.status === 'sent' && quote.signature.canSign ? (
                    <>
                      <p className="field-hint">Signing is available through {formatSignatureTime(quote.signature.signingExpiresAt)}. The certificate binds your typed name and consent to the PDF digest shown above.</p>
                      <form className="auth-form" aria-label="Sign finalized quote" onSubmit={(event) => updateSignature(event, 'signature')}>
                        <label className="field">
                          <span className="field-label">Type the recipient name exactly: {quote.signature.signerName}</span>
                          <input className="text-input" autoComplete="name" maxLength={200} name="signerName" required />
                        </label>
                        <label className="checkbox-row">
                          <input type="checkbox" name="consent" required />
                          <span>{quote.signature.consentText}</span>
                        </label>
                        <Button type="submit" disabled={signatureAction === 'signature'}>{signatureAction === 'signature' ? 'Signing…' : 'Sign quote'}</Button>
                      </form>
                      <details>
                        <summary>Decline this quote</summary>
                        <form className="auth-form" aria-label="Decline finalized quote" onSubmit={(event) => updateSignature(event, 'decline')}>
                          <label className="field">
                            <span className="field-label">Reason for sender (optional)</span>
                            <textarea className="text-input" maxLength={1000} name="reason" rows="3" />
                          </label>
                          <Button className="button-danger" type="submit" disabled={signatureAction === 'decline'}>{signatureAction === 'decline' ? 'Declining…' : 'Decline quote'}</Button>
                        </form>
                      </details>
                    </>
                  ) : null}
                  <p className="field-hint">Open CRM records email-link possession, typed name, explicit consent, time, and immutable document digests. It does not collect your IP address or browser fingerprint. Enforceability depends on the agreement and applicable law.</p>
                </section>
              ) : null}
              {error ? <p className="form-error" role="alert">{error}</p> : null}
              <p className="inline-note"><strong>Receipt is not acceptance.</strong> Confirming receipt only tells the sender that you received this quote. It does not sign, approve, or accept its terms.</p>
            </div>
          </Card>
        ) : null}
      </section>
    </main>
  )
}

import { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { confirmPublicDealQuoteReceipt, getPublicDealQuote, publicQuotePDFURL } from '../lib/deals'
import { usePageTitle } from '../lib/use_page_title'
import { formatMoney, formatSignatureTime } from './deal_view'

export function PublicQuoteRoute() {
  const [params] = useSearchParams()
  const token = params.get('token')?.trim() || ''
  const [quote, setQuote] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isConfirming, setIsConfirming] = useState(false)
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
              {error ? <p className="form-error" role="alert">{error}</p> : null}
              <p className="inline-note"><strong>Receipt is not acceptance.</strong> Confirming receipt only tells the sender that you received this quote. It does not sign, approve, or accept its terms.</p>
            </div>
          </Card>
        ) : null}
      </section>
    </main>
  )
}

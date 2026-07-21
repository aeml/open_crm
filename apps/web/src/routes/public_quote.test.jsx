import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('public quote receipt', () => {
  it('shows immutable quote evidence and confirms receipt without claiming acceptance', async () => {
    let receiptConfirmedAt = ''
    const quote = () => ({
      organizationName: 'Acme, Inc.', quoteNumber: 'Q-12-V1', dealName: 'Bluebird Rollout', recipientName: 'Ava Stone',
      currency: 'USD', total: '308.00', validUntil: '2026-08-20', terms: 'Net 30. Scope changes require approval.',
      pdfFilename: 'quote-bluebird-rollout-v1.pdf', pdfSha256: 'a'.repeat(64), sentAt: '2026-07-21T12:00:00Z', receiptConfirmedAt
    })
    const fetchMock = vi.fn(async (rawURL, options = {}) => {
      const requestURL = new URL(String(rawURL), 'http://localhost')
      if (requestURL.pathname.endsWith('/auth/me')) {
        return { ok: false, status: 401, json: async () => ({ error: { code: 'UNAUTHORIZED' } }) }
      }
      if (requestURL.pathname.endsWith('/api/public/quotes/secure-customer-token') && (options.method || 'GET') === 'GET') {
        return { ok: true, status: 200, json: async () => ({ data: { quote: quote() } }) }
      }
      if (requestURL.pathname.endsWith('/api/public/quotes/secure-customer-token/receipt') && options.method === 'POST') {
        receiptConfirmedAt = '2026-07-21T12:05:00Z'
        return { ok: true, status: 200, json: async () => ({ data: { quote: quote() } }) }
      }
      throw new Error(`Unexpected fetch: ${options.method || 'GET'} ${requestURL.pathname}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/quote?token=secure-customer-token')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Q-12-V1' })).toBeInTheDocument()
    expect(screen.getByText('$308.00')).toBeInTheDocument()
    expect(screen.getByText(/net 30\. scope changes require approval/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /download finalized pdf/i })).toHaveAttribute('href', 'https://crmserver.mendola.tech/api/public/quotes/secure-customer-token/pdf')
    expect(screen.getByText(/receipt is not acceptance/i)).toBeInTheDocument()
    expect(document.body).not.toHaveTextContent('secure-customer-token')

    fireEvent.click(screen.getByRole('button', { name: /confirm receipt/i }))
    expect(await screen.findByText(/receipt confirmed/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /confirm receipt/i })).not.toBeInTheDocument()
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/public\/quotes\/secure-customer-token\/receipt$/), expect.objectContaining({ method: 'POST' }))
    })
  })

  it('collects explicit typed-name consent and exposes the retained certificate after signing', async () => {
    let signature = {
      status: 'sent', signerName: 'Ava Stone', consentText: 'I agree to use an electronic signature and accept finalized quote Q-12-V1 for USD 308.00, including its terms. Typing the named recipient and selecting Sign quote is my electronic signature.',
      signingExpiresAt: '2026-08-21T00:00:00Z', canSign: true
    }
    const quote = () => ({
      organizationName: 'Acme, Inc.', quoteNumber: 'Q-12-V1', dealName: 'Bluebird Rollout', recipientName: 'Ava Stone',
      currency: 'USD', total: '308.00', validUntil: '2026-08-20', terms: 'Net 30.', pdfFilename: 'quote-bluebird-rollout-v1.pdf',
      pdfSha256: 'a'.repeat(64), sentAt: '2026-07-21T12:00:00Z', signature
    })
    const fetchMock = vi.fn(async (rawURL, options = {}) => {
      const requestURL = new URL(String(rawURL), 'http://localhost')
      if (requestURL.pathname.endsWith('/auth/me')) return { ok: false, status: 401, json: async () => ({ error: { code: 'UNAUTHORIZED' } }) }
      if (requestURL.pathname.endsWith('/api/public/quotes/secure-signing-token') && (options.method || 'GET') === 'GET') {
        return { ok: true, status: 200, json: async () => ({ data: { quote: quote() } }) }
      }
      if (requestURL.pathname.endsWith('/api/public/quotes/secure-signing-token/signature') && options.method === 'POST') {
        const input = JSON.parse(options.body)
        expect(input).toEqual({ signerName: 'Ava Stone', consent: true })
        signature = {
          ...signature, status: 'signed', canSign: false, signedName: input.signerName, signedAt: '2026-07-21T12:08:00Z',
          certificateFilename: 'signature-certificate-q-12-v1.pdf', certificateSha256: 'b'.repeat(64)
        }
        return { ok: true, status: 200, json: async () => ({ data: { quote: quote() } }) }
      }
      throw new Error(`Unexpected fetch: ${options.method || 'GET'} ${requestURL.pathname}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/quote?token=secure-signing-token')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /electronic signature/i })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/type the recipient name exactly: ava stone/i), { target: { value: 'Ava Stone' } })
    fireEvent.click(screen.getByRole('checkbox', { name: /i agree to use an electronic signature/i }))
    fireEvent.click(screen.getByRole('button', { name: /^sign quote$/i }))

    expect(await screen.findByText(/signed by ava stone/i)).toBeInTheDocument()
    expect(screen.getByText('b'.repeat(64), { selector: 'code' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /download signature certificate/i })).toHaveAttribute('href', 'https://crmserver.mendola.tech/api/public/quotes/secure-signing-token/signature-certificate')
    expect(document.body).not.toHaveTextContent('secure-signing-token')
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/public\/quotes\/secure-signing-token\/signature$/), expect.objectContaining({
        method: 'POST', headers: expect.objectContaining({ 'Idempotency-Key': expect.stringMatching(/^quote-signature-/) })
      }))
    })
  })
})

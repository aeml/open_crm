import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DealQuoteVersionsCard } from './deal_quote_versions'

const baseQuote = {
  id: 71,
  version: 1,
  quoteNumber: 'Q-12-V1',
  recipientName: 'Ava Stone',
  recipientEmail: 'ava@example.test',
  currency: 'USD',
  total: '308.00',
  validUntil: '2026-07-01',
  createdAt: '2026-06-01T12:00:00Z',
  createdByUserName: 'Demo Owner',
  pdfSha256: 'a'.repeat(64),
  deliveries: []
}

function renderCard(quote, overrides = {}) {
  const onReissue = vi.fn()
  const view = render(<DealQuoteVersionsCard
    areLineItemsDirty={false}
    canWrite
    deal={{ id: 12, status: 'open' }}
    deliveringQuoteId={null}
    form={{ recipientName: '', recipientEmail: '', validUntil: '2099-12-31', terms: '' }}
    isFinalizing={false}
    isSnapshotPending={false}
    lineItems={[]}
    onDeliver={vi.fn()}
    onFinalize={vi.fn()}
    onReissue={onReissue}
    onResolveDelivery={vi.fn()}
    onSetForm={vi.fn()}
    quotes={[quote]}
    reissuingQuoteId={null}
    resolvingDeliveryId={null}
    signatureRequests={[]}
    {...overrides}
  />)
  return { onReissue, ...view }
}

describe('DealQuoteVersionsCard expiration workflow', () => {
  it('blocks delivery and creates a dated replacement from an expired version', () => {
    const quote = { ...baseQuote, lifecycleStatus: 'expired' }
    const { onReissue } = renderCard(quote)

    expect(screen.getByText('Expired')).toBeInTheDocument()
    expect(screen.getByText(/expired quotes cannot be delivered/i)).toBeInTheDocument()
    expect(screen.queryByText(/deliver by email/i)).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: 'Create replacement quote' }))

    expect(onReissue).toHaveBeenCalledWith(quote, '2099-12-31')
  })

  it('keeps superseded and signed expired evidence read-only', () => {
    const replaced = { ...baseQuote, lifecycleStatus: 'superseded', reissuedByQuoteNumber: 'Q-12-V2' }
    const { unmount } = renderCard(replaced)
    expect(screen.getByText('Replaced')).toBeInTheDocument()
    expect(screen.getByText(/replaced by Q-12-V2/i)).toBeInTheDocument()
    expect(screen.queryByText(/deliver this version/i)).not.toBeInTheDocument()
    unmount()

    renderCard({ ...baseQuote, lifecycleStatus: 'expired' }, {
      signatureRequests: [{ id: 44, quoteId: 71, status: 'signed' }]
    })
    expect(screen.getByText(/signed evidence requires/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Create replacement quote' })).not.toBeInTheDocument()
  })

  it('keeps expired quotes on closed deals read-only', () => {
    renderCard({ ...baseQuote, lifecycleStatus: 'expired' }, { deal: { id: 12, status: 'won' } })
    expect(screen.getByText(/reopen the deal before reissuing/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Create replacement quote' })).not.toBeInTheDocument()
  })
})

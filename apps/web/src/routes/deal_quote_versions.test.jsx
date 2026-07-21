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
	it('uses retained template delivery defaults only after independent approval', () => {
	  const onDecideApproval = vi.fn()
	  const pending = {
	    ...baseQuote,
	    createdByUserId: 1,
	    template: { id: 4, name: 'Services MSA', revision: 2 },
	    deliveryDefaults: { subject: 'Review Q-12-V1', messageBody: 'Please review retained terms.', requestSignature: true },
	    approval: { required: true, status: 'pending', requestedByUserId: 1, requestedByUserName: 'Demo Owner', requestedAt: '2026-06-01T12:00:00Z' }
	  }
	  const { rerender } = renderCard(pending, { canAdminister: true, currentUserId: '2', onDecideApproval })

	  expect(screen.getByText(/retained revision 2/i)).toBeInTheDocument()
	  expect(screen.getByText(/delivery blocked until independent approval/i)).toBeInTheDocument()
	  expect(screen.queryByText('Deliver by email')).not.toBeInTheDocument()
	  fireEvent.click(screen.getByRole('button', { name: 'Approve exact PDF' }))
	  expect(onDecideApproval).toHaveBeenCalledWith(pending, 'approved', '')

	  const approved = { ...pending, approval: { ...pending.approval, status: 'approved', decidedByUserId: 2, decidedByUserName: 'Alex Admin', decidedAt: '2026-06-01T13:00:00Z' } }
	  rerender(<DealQuoteVersionsCard
	    areLineItemsDirty={false}
	    canAdminister
	    canWrite
	    currentUserId="2"
	    deal={{ id: 12, status: 'open' }}
	    form={{ recipientName: '', recipientEmail: '', validUntil: '2099-12-31', terms: '', templateId: '' }}
	    isFinalizing={false}
	    isSnapshotPending={false}
	    lineItems={[]}
	    onDecideApproval={onDecideApproval}
	    onDeliver={vi.fn()}
	    onFinalize={vi.fn()}
	    onReissue={vi.fn()}
	    onResolveDelivery={vi.fn()}
	    onSetForm={vi.fn()}
	    quotes={[approved]}
	    resolvingDeliveryId={null}
	    signatureRequests={[]}
	  />)
	  expect(screen.getByText('Deliver by email')).toBeInTheDocument()
	  expect(screen.getByLabelText(/email subject for Q-12-V1/i)).toHaveValue('Review Q-12-V1')
	  expect(screen.getByLabelText(/message for Q-12-V1/i)).toHaveValue('Please review retained terms.')
	  expect(screen.getByRole('checkbox', { name: /request signature from Ava Stone/i })).toBeChecked()
	})

  it('prevents the quote creator from deciding their own pending approval', () => {
    renderCard({
      ...baseQuote,
      createdByUserId: 1,
      approval: { required: true, status: 'pending', requestedByUserId: 1, requestedByUserName: 'Demo Owner' }
    }, { canAdminister: true, currentUserId: '1', onDecideApproval: vi.fn() })

    expect(screen.queryByRole('button', { name: 'Approve exact PDF' })).not.toBeInTheDocument()
    expect(screen.getByText(/different active owner or admin/i)).toBeInTheDocument()
  })

	it('shows the immutable reporting-currency snapshot and preserves the customer amount', () => {
		renderCard({
		  ...baseQuote,
		  currency: 'EUR',
		  fxDisclosure: { baseCurrency: 'USD', rateToBase: '1.10000000', effectiveDate: '2026-06-01', source: 'ECB reference', totalInBaseCurrency: '338.80', displayText: 'USD 338.80 reporting equivalent at 1 EUR = 1.10000000 USD (ECB reference, effective 2026-06-01). Customer amount remains EUR 308.00.' }
		})
		expect(screen.getByText(/USD 338\.80 reporting equivalent/i)).toHaveTextContent('1 EUR = 1.10000000 USD')
		expect(screen.getByText(/USD 338\.80 reporting equivalent/i)).toHaveTextContent('Customer amount remains EUR 308.00')
	})

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

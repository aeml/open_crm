import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  deliverDealQuote,
  finalizeDealQuote,
  replaceDealLineItems,
  resolveDealQuoteDelivery,
  voidDealSignatureRequest
} from '../lib/deals'
import { useDealCommercials } from './use_deal_commercials'
import { useDealSelection } from './use_deal_selection'

vi.mock('../lib/deals', () => ({
  deliverDealQuote: vi.fn(),
  finalizeDealQuote: vi.fn(),
  replaceDealLineItems: vi.fn(),
  resolveDealQuoteDelivery: vi.fn(),
  voidDealSignatureRequest: vi.fn()
}))

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useDealCommercials', () => {
  it('reuses the finalization key for an identical retry and records one returned quote', async () => {
    const finalized = { id: 71, quoteNumber: 'Q-11-V1', recipientName: 'Ava' }
    finalizeDealQuote.mockRejectedValueOnce(new Error('Network failed')).mockResolvedValueOnce(finalized)
    const onError = vi.fn()
    const { result } = renderHook(() => {
      const selection = useDealSelection(11)
      return useDealCommercials({ selectedDealId: 11, selection, onDealUpdated: vi.fn(), onError })
    })

    act(() => {
      result.current.load({ deal: { id: 11, primaryContactName: 'Ava' }, lineItems: [{ id: 9 }], quotes: [] })
      result.current.setQuoteForm({ recipientName: 'Ava', recipientEmail: 'ava@example.test', validUntil: '2026-08-20', terms: 'Net 30' })
    })
    await act(async () => result.current.handleFinalizeQuote({ preventDefault: vi.fn() }))
    await act(async () => result.current.handleFinalizeQuote({ preventDefault: vi.fn() }))

    expect(finalizeDealQuote).toHaveBeenCalledTimes(2)
    expect(finalizeDealQuote.mock.calls[0][2]).toBe(finalizeDealQuote.mock.calls[1][2])
    expect(result.current.quotes).toEqual([finalized])
    expect(onError).toHaveBeenCalledWith('Network failed')
    expect(onError).toHaveBeenLastCalledWith('')
  })

  it('keeps late quote changes off the active deal and rejects mismatched snapshots', async () => {
    const lineItemSave = deferred()
    replaceDealLineItems.mockReturnValue(lineItemSave.promise)
    const onDealUpdated = vi.fn()
    const onError = vi.fn()
    const { result, rerender } = renderHook(({ dealId }) => {
      const selection = useDealSelection(dealId)
      const commercials = useDealCommercials({ selectedDealId: dealId, selection, onDealUpdated, onError })
      return { commercials, selection }
    }, { initialProps: { dealId: 11 } })

    let firstSave
    act(() => {
      firstSave = result.current.commercials.handleSaveLineItems()
      result.current.commercials.handleSaveLineItems()
    })
    expect(replaceDealLineItems).toHaveBeenCalledTimes(1)
    expect(result.current.commercials.isSavingLineItems).toBe(true)
    expect(result.current.commercials.isSnapshotPending).toBe(true)

    act(() => {
      result.current.selection.begin(12)
      rerender({ dealId: 12 })
      result.current.commercials.reset()
    })
    await act(async () => {
      lineItemSave.resolve({ deal: { id: 11 }, lineItems: [{ id: 71 }], activities: [{ id: 81 }] })
      await firstSave
    })
    expect(onDealUpdated).toHaveBeenCalledWith(expect.objectContaining({ deal: { id: 11 } }), 11, false)
    expect(result.current.commercials.lineItems).toEqual([])
    expect(result.current.commercials.isSnapshotPending).toBe(false)
    expect(onError).not.toHaveBeenCalled()
  })

  it('reuses uncertain delivery keys and rotates them after terminal resolution', async () => {
    const quote = { id: 71, quoteNumber: 'Q-11-V1', deliveries: [] }
    const uncertain = { id: 81, quoteId: 71, status: 'uncertain', accessCount: 0, downloadCount: 0, lastError: 'Check Sent' }
    const failed = { ...uncertain, status: 'failed', lastError: 'Confirmed not sent' }
    const sent = { id: 82, quoteId: 71, status: 'sent', accessCount: 0, downloadCount: 0 }
    deliverDealQuote.mockRejectedValueOnce(new Error('Network failed')).mockResolvedValueOnce(uncertain).mockResolvedValueOnce(sent)
    resolveDealQuoteDelivery.mockResolvedValue(failed)
    const onError = vi.fn()
    const { result } = renderHook(() => {
      const selection = useDealSelection(11)
      return useDealCommercials({ selectedDealId: 11, selection, onDealUpdated: vi.fn(), onError })
    })

    act(() => result.current.load({ deal: { id: 11 }, quotes: [quote] }))
    const input = { subject: 'Finalized quote', messageBody: 'Please review.' }
    await act(async () => result.current.handleDeliverQuote(quote, input))
    await act(async () => result.current.handleDeliverQuote(quote, input))

    expect(deliverDealQuote).toHaveBeenCalledTimes(2)
    expect(deliverDealQuote.mock.calls[0][3]).toBe(deliverDealQuote.mock.calls[1][3])
    const uncertainKey = deliverDealQuote.mock.calls[1][3]
    expect(result.current.quotes[0].deliveries).toEqual([uncertain])

    await act(async () => result.current.handleResolveQuoteDelivery(71, 81, 'not_sent'))
    expect(resolveDealQuoteDelivery).toHaveBeenCalledWith(81, 'not_sent')
    expect(result.current.quotes[0].deliveries[0]).toEqual(failed)

    await act(async () => result.current.handleDeliverQuote(quote, input))
    expect(deliverDealQuote).toHaveBeenCalledTimes(3)
    expect(deliverDealQuote.mock.calls[2][3]).not.toBe(uncertainKey)
    expect(result.current.quotes[0].deliveries).toEqual([sent, failed])
    expect(onError).toHaveBeenLastCalledWith('')
  })

  it('creates delivery-bound signature evidence and voids it through a deal snapshot', async () => {
    const quote = {
      id: 71,
      quoteNumber: 'Q-11-V1',
      recipientName: 'Ava Stone',
      recipientEmail: 'ava@example.test',
      pdfFilename: 'quote-q-11-v1.pdf',
      deliveries: []
    }
    const delivery = {
      id: 81,
      quoteId: 71,
      signatureRequestId: 91,
      status: 'sent',
      sentAt: '2026-07-21T03:05:00Z',
      createdAt: '2026-07-21T03:05:00Z',
      updatedAt: '2026-07-21T03:05:00Z'
    }
    deliverDealQuote.mockResolvedValue(delivery)
    voidDealSignatureRequest.mockResolvedValue({
      deal: { id: 11 },
      quotes: [{ ...quote, deliveries: [delivery] }],
      signatureRequests: [{
        id: 91,
        provider: 'open_crm_native',
        signerName: 'Ava Stone',
        status: 'voided'
      }]
    })
    const onDealUpdated = vi.fn()
    const onError = vi.fn()
    const { result } = renderHook(() => {
      const selection = useDealSelection(11)
      return useDealCommercials({ selectedDealId: 11, selection, onDealUpdated, onError })
    })

    act(() => result.current.load({ deal: { id: 11 }, quotes: [quote], signatureRequests: [] }))
    await act(async () => result.current.handleDeliverQuote(quote, {
      subject: 'Please sign Q-11-V1',
      messageBody: 'Review and sign.',
      requestSignature: true
    }))

    expect(deliverDealQuote).toHaveBeenCalledWith(11, 71, {
      subject: 'Please sign Q-11-V1',
      messageBody: 'Review and sign.',
      requestSignature: true
    }, expect.stringMatching(/^quote-delivery-/))
    expect(result.current.signatureRequests).toEqual([
      expect.objectContaining({
        id: 91,
        quoteId: 71,
        deliveryId: 81,
        provider: 'open_crm_native',
        status: 'sent'
      })
    ])

    await act(async () => result.current.handleVoidSignatureRequest(91))

    expect(voidDealSignatureRequest).toHaveBeenCalledWith(11, 91)
    expect(result.current.signatureRequests[0].status).toBe('voided')
    expect(onDealUpdated).toHaveBeenCalledWith(expect.objectContaining({ deal: { id: 11 } }), 11, true)
    expect(onError).toHaveBeenLastCalledWith('')
  })
})

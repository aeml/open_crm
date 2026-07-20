import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createDealSignatureRequest, replaceDealLineItems } from '../lib/deals'
import { useDealCommercials } from './use_deal_commercials'
import { useDealSelection } from './use_deal_selection'

vi.mock('../lib/deals', () => ({
  createDealSignatureRequest: vi.fn(),
  replaceDealLineItems: vi.fn(),
  updateDealSignatureRequestStatus: vi.fn()
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

    createDealSignatureRequest.mockResolvedValue({ deal: { id: 11 }, signatureRequests: [] })
    act(() => result.current.commercials.setSignatureForm({ signerName: 'Ava', signerEmail: 'ava@example.test' }))
    await act(async () => {
      await result.current.commercials.handleCreateSignatureRequest({ preventDefault: vi.fn() })
    })
    expect(onDealUpdated).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledWith('Unable to create proposal tracking.')
    await waitFor(() => expect(result.current.commercials.isCreatingSignatureRequest).toBe(false))
  })
})

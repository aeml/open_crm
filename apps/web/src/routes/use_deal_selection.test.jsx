import { act, renderHook } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { requireDealResponse, useDealSelection } from './use_deal_selection'

describe('useDealSelection', () => {
  it('distinguishes repeated visits, aborts old loads, and suppresses duplicate mutations', () => {
    const { result } = renderHook(() => useDealSelection(null))
    let firstAlpha
    let alphaUpdate

    act(() => {
      firstAlpha = result.current.begin(11)
      alphaUpdate = result.current.start('update', 11, { group: 'deal-snapshot' })
    })
    expect(result.current.start('stage', 11, { group: 'deal-snapshot' })).toBeNull()

    let beta
    let secondAlpha
    act(() => {
      result.current.finish(alphaUpdate)
      beta = result.current.begin(12)
      secondAlpha = result.current.begin(11)
    })

    expect(firstAlpha.controller.signal.aborted).toBe(true)
    expect(beta.controller.signal.aborted).toBe(true)
    expect(result.current.isCurrent(firstAlpha)).toBe(false)
    expect(result.current.isCurrent(secondAlpha)).toBe(true)
    expect(result.current.canApply({ dealId: 11, selection: firstAlpha })).toBe(false)
    expect(result.current.canApply({ dealId: 12, selection: beta })).toBe(true)
    expect(result.current.start('update', 12)).toBeNull()
  })

  it('rejects missing and mismatched deal snapshots', () => {
    expect(() => requireDealResponse({}, 11)).toThrow('Unable to update deal.')
    expect(() => requireDealResponse({ deal: { id: 12 } }, 11, 'Unable to load deal.')).toThrow('Unable to load deal.')
    expect(requireDealResponse({ deal: { id: 11 } }, 11).deal.id).toBe(11)
  })
})

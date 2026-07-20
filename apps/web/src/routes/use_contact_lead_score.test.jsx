import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { evaluateContactLeadScore } from '../lib/lead_scoring'
import { useContactLeadScore } from './use_contact_lead_score'

vi.mock('../lib/lead_scoring', () => ({ evaluateContactLeadScore: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => {
    resolve = next
  })
  return { promise, resolve }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useContactLeadScore', () => {
  it('discards a late evaluation after leaving and returning to a contact', async () => {
    const evaluation = deferred()
    evaluateContactLeadScore.mockReturnValue(evaluation.promise)
    const onScored = vi.fn()
    const onError = vi.fn()
    const { result, rerender } = renderHook(
      ({ contactId }) => useContactLeadScore({ selectedContactId: contactId, onScored, onError }),
      { initialProps: { contactId: 7 } }
    )

    let evaluationPromise
    act(() => {
      evaluationPromise = result.current.handleEvaluateLeadScore()
    })
    await waitFor(() => {
      expect(evaluateContactLeadScore).toHaveBeenCalledWith(7)
      expect(result.current.isEvaluatingLeadScore).toBe(true)
    })

    rerender({ contactId: 8 })
    rerender({ contactId: 7 })

    await act(async () => {
      evaluation.resolve({ contact: { id: 7 }, score: 50, matchedRules: [{ id: 1 }] })
      await evaluationPromise
    })

    expect(onScored).not.toHaveBeenCalled()
    expect(onError).not.toHaveBeenCalled()
    expect(result.current.leadScoreStatus).toBe('')
    expect(result.current.isEvaluatingLeadScore).toBe(false)
  })

  it('prevents duplicate in-flight evaluation and publishes a scoped result', async () => {
    const evaluation = deferred()
    evaluateContactLeadScore.mockReturnValue(evaluation.promise)
    const onScored = vi.fn()
    const onError = vi.fn()
    const { result } = renderHook(() => useContactLeadScore({ selectedContactId: 7, onScored, onError }))

    let evaluationPromise
    act(() => {
      evaluationPromise = result.current.handleEvaluateLeadScore()
      result.current.handleEvaluateLeadScore()
    })
    expect(evaluateContactLeadScore).toHaveBeenCalledTimes(1)

    await act(async () => {
      evaluation.resolve({
        contact: { id: 7, firstName: 'Riley' },
        score: 72,
        grade: 'B',
        matchedRules: [{ id: 1 }, { id: 2 }],
        assignedToUserName: 'Alex Owner'
      })
      await evaluationPromise
    })

    expect(onScored).toHaveBeenCalledWith(expect.objectContaining({ id: 7 }), 7)
    expect(onError).toHaveBeenLastCalledWith('')
    expect(result.current.leadScoreStatus).toBe('Lead scored 72 (B); 2 rules matched. Routed to Alex Owner.')
    expect(result.current.isEvaluatingLeadScore).toBe(false)
  })

  it('rejects a response for the wrong contact', async () => {
    evaluateContactLeadScore.mockResolvedValue({ contact: { id: 8 }, score: 99, matchedRules: [] })
    const onScored = vi.fn()
    const onError = vi.fn()
    const { result } = renderHook(() => useContactLeadScore({ selectedContactId: 7, onScored, onError }))

    await act(async () => {
      await result.current.handleEvaluateLeadScore()
    })

    expect(onScored).not.toHaveBeenCalled()
    expect(onError).toHaveBeenCalledWith('Unable to evaluate lead score.')
    expect(result.current.leadScoreStatus).toBe('')
  })
})

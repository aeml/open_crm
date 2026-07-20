import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createCompanyLinkedPerson } from '../lib/companies'
import { useCompanyPeople } from './use_company_people'

vi.mock('../lib/companies', () => ({ createCompanyLinkedPerson: vi.fn() }))

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

describe('useCompanyPeople', () => {
  it('discards a late create response after leaving and returning to a company', async () => {
    const createResult = deferred()
    createCompanyLinkedPerson.mockReturnValue(createResult.promise)
    const onCreated = vi.fn()
    const onError = vi.fn()
    const company6 = { id: 6, clientType: 'organization', status: 'prospect' }
    const company7 = { id: 7, clientType: 'organization', status: 'prospect' }
    const { result, rerender } = renderHook(
      ({ selectedCompany }) => useCompanyPeople({
        selectedCompanyId: selectedCompany.id,
        selectedCompany,
        customDefinitions: [],
        onCreated,
        onError
      }),
      { initialProps: { selectedCompany: company6 } }
    )

    act(() => {
      result.current.setForm((current) => ({ ...current, firstName: 'Riley', lastName: 'Chen' }))
    })
    let submitPromise
    act(() => {
      submitPromise = result.current.handleSubmit({ preventDefault: vi.fn() })
    })
    await waitFor(() => {
      expect(createCompanyLinkedPerson).toHaveBeenCalledWith(6, expect.objectContaining({ firstName: 'Riley', lastName: 'Chen' }))
    })

    rerender({ selectedCompany: company7 })
    rerender({ selectedCompany: company6 })

    await act(async () => {
      createResult.resolve({ contact: { id: 9, firstName: 'Riley', lastName: 'Chen' }, link: { relationshipTitle: '', isPrimary: false } })
      await submitPromise
    })

    expect(onCreated).not.toHaveBeenCalled()
    expect(onError).not.toHaveBeenCalled()
    expect(result.current.isSaving).toBe(false)
    expect(result.current.showForm).toBe(false)
  })
})

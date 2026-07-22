import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createCompanyLinkedPerson, linkCompanyContact, unlinkCompanyContact } from '../lib/companies'
import { useCompanyPeople } from './use_company_people'

vi.mock('../lib/companies', () => ({
  createCompanyLinkedPerson: vi.fn(),
  linkCompanyContact: vi.fn(),
  unlinkCompanyContact: vi.fn()
}))

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

afterEach(() => {
  vi.restoreAllMocks()
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

  it('serializes an existing-contact link and refreshes the selected company once', async () => {
    const linkResult = deferred()
    linkCompanyContact.mockReturnValue(linkResult.promise)
    const onRelationshipsChanged = vi.fn().mockResolvedValue(true)
	const selectedCompany = { id: 6, clientType: 'organization', status: 'prospect' }
    const { result } = renderHook(() => useCompanyPeople({
      selectedCompanyId: 6,
	  selectedCompany,
      customDefinitions: [],
      onCreated: vi.fn(),
      onError: vi.fn(),
      onRelationshipsChanged
    }))

    act(() => {
      result.current.setLinkForm({ contactId: '9', relationshipTitle: 'Buyer', isPrimary: true })
    })
    let firstSubmit
    act(() => {
      firstSubmit = result.current.handleLinkSubmit({ preventDefault: vi.fn() })
      result.current.handleLinkSubmit({ preventDefault: vi.fn() })
    })
    expect(linkCompanyContact).toHaveBeenCalledTimes(1)
    expect(linkCompanyContact).toHaveBeenCalledWith(6, 9, { relationshipTitle: 'Buyer', isPrimary: true })

    await act(async () => {
      linkResult.resolve({ linkedContact: { id: 9, isPrimary: true } })
      await firstSubmit
    })
    expect(onRelationshipsChanged).toHaveBeenCalledTimes(1)
    expect(result.current.isLinking).toBe(false)
    expect(result.current.showLinkForm).toBe(false)
  })

  it('promotes and unlinks people through the bounded relationship API', async () => {
    linkCompanyContact.mockResolvedValue({ linkedContact: { id: 9, isPrimary: true } })
    unlinkCompanyContact.mockResolvedValue(undefined)
    const onRelationshipsChanged = vi.fn().mockResolvedValue(true)
    const onError = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
	const selectedCompany = { id: 6, clientType: 'organization', status: 'prospect' }
    const { result } = renderHook(() => useCompanyPeople({
      selectedCompanyId: 6,
	  selectedCompany,
      customDefinitions: [],
      onCreated: vi.fn(),
      onError,
      onRelationshipsChanged
    }))
    const contact = { id: 9, firstName: 'Riley', lastName: 'Chen', relationshipTitle: 'Buyer', isPrimary: false }

    await act(async () => {
      await result.current.handleMakePrimary(contact)
    })
    expect(linkCompanyContact).toHaveBeenCalledWith(6, 9, { relationshipTitle: 'Buyer', isPrimary: true })

    await act(async () => {
      await result.current.handleUnlink(contact)
    })
    expect(unlinkCompanyContact).toHaveBeenCalledWith(6, 9)
    expect(onRelationshipsChanged).toHaveBeenCalledTimes(2)
    expect(onError).not.toHaveBeenCalled()
  })
})

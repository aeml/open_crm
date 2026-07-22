import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { listCompanyLinkedContacts } from '../lib/companies'
import { useCompanyLinkedContacts } from './use_company_linked_contacts'

vi.mock('../lib/companies', () => ({ listCompanyLinkedContacts: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

function contact(id, name, primary = false) {
  return { id, firstName: name, lastName: 'Person', email: `${name.toLowerCase()}@example.test`, relationshipTitle: '', isPrimary: primary }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useCompanyLinkedContacts', () => {
  it('loads stable continuation and search pages without replacing the primary context', async () => {
    const firstPage = [contact(1, 'Primary', true), contact(2, 'Second')]
    listCompanyLinkedContacts.mockImplementation((_companyId, query) => {
      if (query.search === 'remote') {
        return Promise.resolve({ linkedContacts: [contact(51, 'Remote')], meta: { page: 1, pageSize: 50, total: 1 } })
      }
      if (query.page === 2) {
        return Promise.resolve({ linkedContacts: [contact(51, 'Last')], meta: { page: 2, pageSize: 50, total: 51 } })
      }
      return Promise.resolve({ linkedContacts: firstPage, meta: { page: 1, pageSize: 50, total: 51 } })
    })
    const initialMeta = { page: 1, pageSize: 50, total: 51 }
    const { result } = renderHook(() => useCompanyLinkedContacts({
      companyId: 6,
      initialContacts: firstPage,
      initialMeta
    }))

    await act(async () => {
      await result.current.loadMore()
    })
    expect(result.current.contacts.map((entry) => entry.id)).toEqual([1, 2, 51])
    expect(result.current.meta).toMatchObject({ page: 2, total: 51 })

    act(() => result.current.setQuery('remote'))
    await act(async () => {
      await result.current.search()
    })
    expect(result.current.contacts.map((entry) => entry.id)).toEqual([51])
    expect(result.current.primaryContact?.id).toBe(1)
    expect(result.current.unfilteredContacts.map((entry) => entry.id)).toEqual([1, 2, 51])
    expect(result.current.knownContacts.map((entry) => entry.id)).toEqual([1, 2, 51])

    await act(async () => {
      await result.current.refresh()
    })
    expect(result.current.appliedQuery).toBe('')
    expect(result.current.contacts.map((entry) => entry.id)).toEqual([1, 2])
    expect(listCompanyLinkedContacts).toHaveBeenLastCalledWith(6, { search: '', page: 1, pageSize: 50 }, expect.any(Object))
  })

  it('discards a late A-to-B-to-A search response', async () => {
    const stale = deferred()
    listCompanyLinkedContacts.mockReturnValue(stale.promise)
    const company6 = [contact(1, 'Six primary', true)]
    const company7 = [contact(7, 'Seven primary', true)]
    const initialMeta = { page: 1, pageSize: 50, total: 1 }
    const { result, rerender } = renderHook(
      ({ companyId, initialContacts }) => useCompanyLinkedContacts({
        companyId,
        initialContacts,
        initialMeta
      }),
      { initialProps: { companyId: 6, initialContacts: company6 } }
    )

    act(() => result.current.setQuery('stale'))
    let searchPromise
    act(() => {
      searchPromise = result.current.search()
    })
    rerender({ companyId: 7, initialContacts: company7 })
    rerender({ companyId: 6, initialContacts: company6 })

    await act(async () => {
      stale.resolve({ linkedContacts: [contact(99, 'Stale')], meta: { page: 1, pageSize: 50, total: 1 } })
      await searchPromise
    })
    await waitFor(() => expect(result.current.contacts.map((entry) => entry.id)).toEqual([1]))
    expect(result.current.appliedQuery).toBe('')
  })
})

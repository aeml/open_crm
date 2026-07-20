import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listCustomFields } from '../lib/custom_fields'
import { listOrganizationUsers } from '../lib/users'
import { buildCompaniesPath, useCompanyDirectory } from './use_company_directory'

vi.mock('../lib/companies', () => ({ listCompanies: vi.fn() }))
vi.mock('../lib/contacts', () => ({ listContacts: vi.fn() }))
vi.mock('../lib/custom_fields', () => ({ listCustomFields: vi.fn() }))
vi.mock('../lib/users', () => ({ listOrganizationUsers: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

function company(id, name) {
  return { id, name, status: 'prospect', customFields: {} }
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('company directory state', () => {
  it('builds a shareable path from every active directory filter', () => {
    expect(buildCompaniesPath('north star', '7', { fieldKey: 'tier', operator: 'eq', value: 'gold' }))
      .toBe('/companies?q=north+star&owner=7&customField=tier&customOperator=eq&customValue=gold')
  })

  it('does not let a late initial load replace a newer search result', async () => {
    const initialCompanies = deferred()
    listCompanies.mockImplementation(({ search }) => search === 'new' ? Promise.resolve({ companies: [company(2, 'New result')] }) : initialCompanies.promise)
    listContacts.mockResolvedValue({ contacts: [] })
    listOrganizationUsers.mockResolvedValue([])
    listCustomFields.mockResolvedValue([])

    const { result } = renderHook(() => useCompanyDirectory({
      initialCustomFilter: { fieldKey: '', operator: '', value: '' },
      initialOwnerFilter: 'all',
      initialSearch: ''
    }))

    await act(async () => {
      await result.current.reloadCompanies('new', 'all', { fieldKey: '', operator: '', value: '' })
    })
    expect(result.current.companies.map((entry) => entry.name)).toEqual(['New result'])

    await act(async () => {
      initialCompanies.resolve({ companies: [company(1, 'Stale result')] })
      await initialCompanies.promise
    })
    await waitFor(() => expect(result.current.isListLoading).toBe(false))
    expect(result.current.companies.map((entry) => entry.name)).toEqual(['New result'])
  })
})

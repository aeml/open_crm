import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listDeals, listDealPipelines } from '../lib/deals'
import { listOrganizationUsers } from '../lib/users'
import { dealDirectoryPath, useDealDirectory } from './use_deal_directory'

vi.mock('../lib/companies', () => ({ listCompanies: vi.fn() }))
vi.mock('../lib/contacts', () => ({ listContacts: vi.fn() }))
vi.mock('../lib/deals', () => ({ listDeals: vi.fn(), listDealPipelines: vi.fn() }))
vi.mock('../lib/users', () => ({ listOrganizationUsers: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

function deal(id, name) {
  return { id, name, stageId: 1, status: 'open' }
}

const initialForm = {
  name: '',
  stageId: '',
  companyId: '',
  primaryContactId: '',
  valueAmount: '',
  valueCurrency: 'USD',
  expectedCloseDate: '',
  ownerUserId: ''
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('deal directory state', () => {
  it('builds a shareable detail path from every active filter', () => {
    expect(dealDirectoryPath({ dealId: 9, search: 'renewal', pipeline: '2', stage: '4', owner: '7', closeFrom: '2026-08-01', closeTo: '2026-08-31' }))
      .toBe('/deals/9?q=renewal&pipeline=2&stage=4&owner=7&closeFrom=2026-08-01&closeTo=2026-08-31')
  })

  it('does not let a late bootstrap load replace a newer filtered result', async () => {
    const initialDeals = deferred()
    listDeals.mockImplementation(({ search }) => search === 'new' ? Promise.resolve({ deals: [deal(2, 'New result')] }) : initialDeals.promise)
    listDealPipelines.mockResolvedValue([{ id: 1, name: 'Sales', stages: [{ id: 1, name: 'Open', position: 1 }] }])
    listCompanies.mockResolvedValue({ companies: [] })
    listContacts.mockResolvedValue({ contacts: [] })
    listOrganizationUsers.mockResolvedValue([])

    const { result } = renderHook(() => useDealDirectory({
      initialCloseFrom: '',
      initialCloseTo: '',
      initialCompanyId: '',
      initialForm,
      initialOwnerFilter: 'all',
      initialPipelineFilter: 'all',
      initialPrimaryContactId: '',
      initialSearch: '',
      initialStageFilter: 'all',
      routeDealId: 0
    }))

    await act(async () => {
      await result.current.reloadDeals('new', 'all', 'all', 'all', '', '')
    })
    expect(result.current.deals.map((entry) => entry.name)).toEqual(['New result'])

    await act(async () => {
      initialDeals.resolve({ deals: [deal(1, 'Stale result')] })
      await initialDeals.promise
    })
    await waitFor(() => expect(result.current.pipelineReady).toBe(true))
    expect(result.current.deals.map((entry) => entry.name)).toEqual(['New result'])
  })

  it('synchronizes list state when browser history changes the filter query', async () => {
    listDeals.mockImplementation(({ search }) => Promise.resolve({ deals: [deal(search ? 2 : 1, search ? 'History result' : 'Initial result')] }))
    listDealPipelines.mockResolvedValue([{ id: 1, name: 'Sales', stages: [{ id: 1, name: 'Open', position: 1 }] }])
    listCompanies.mockResolvedValue({ companies: [] })
    listContacts.mockResolvedValue({ contacts: [] })
    listOrganizationUsers.mockResolvedValue([])

    const { result, rerender } = renderHook(({ query }) => useDealDirectory({
      initialCloseFrom: '',
      initialCloseTo: '',
      initialCompanyId: '',
      initialForm,
      initialOwnerFilter: 'all',
      initialPipelineFilter: 'all',
      initialPrimaryContactId: '',
      initialSearch: query,
      initialStageFilter: 'all',
      routeDealId: 0
    }), { initialProps: { query: '' } })

    await waitFor(() => expect(result.current.deals.map((entry) => entry.name)).toEqual(['Initial result']))
    rerender({ query: 'history' })

    await waitFor(() => expect(result.current.deals.map((entry) => entry.name)).toEqual(['History result']))
    expect(result.current.search).toBe('history')
  })
})

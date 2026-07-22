import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'
import { taskDirectoryPath, useTaskDirectory } from './use_task_directory'

vi.mock('../lib/companies', () => ({ listCompanies: vi.fn() }))
vi.mock('../lib/contacts', () => ({ listContacts: vi.fn() }))
vi.mock('../lib/deals', () => ({ listDeals: vi.fn() }))
vi.mock('../lib/tasks', () => ({ listTasks: vi.fn() }))
vi.mock('../lib/users', () => ({ listOrganizationUsers: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => { resolve = next })
  return { promise, resolve }
}

function task(id, title) {
  return { id, title, status: 'open', entityType: 'deal', entityId: 1 }
}

function hookProps(overrides = {}) {
  return {
    initialAssigneeFilter: 'all',
    initialDueView: 'all',
    initialEntityIdFilter: '',
    initialEntityTypeFilter: 'all',
    initialSearch: '',
    initialStatusFilter: 'open',
    navigate: vi.fn(),
    routeTaskId: 0,
    ...overrides
  }
}

function resolveOptions() {
  listDeals.mockResolvedValue({ deals: [] })
  listCompanies.mockResolvedValue({ companies: [] })
  listContacts.mockResolvedValue({ contacts: [] })
  listOrganizationUsers.mockResolvedValue([])
}

afterEach(() => {
  vi.clearAllMocks()
})

describe('task directory state', () => {
  it('builds a shareable detail path from every active filter', () => {
    expect(taskDirectoryPath({ taskId: 9, search: 'renewal', status: 'completed', due: 'overdue', assignee: '7', entityType: 'company', entityId: '4' }))
      .toBe('/tasks/9?q=renewal&status=completed&due=overdue&assignee=7&entityType=company&entityId=4')
  })

  it('does not let a late bootstrap load replace a newer filtered result', async () => {
    const initialTasks = deferred()
    listTasks.mockImplementation(({ search }) => search === 'new' ? Promise.resolve({ tasks: [task(2, 'New result')] }) : initialTasks.promise)
    resolveOptions()
    const { result } = renderHook(() => useTaskDirectory(hookProps()))

    await act(async () => {
      await result.current.reloadTasks('new', 'open', 'all', '', 'all', 'all')
    })
    expect(result.current.tasks.map((entry) => entry.title)).toEqual(['New result'])

    await act(async () => {
      initialTasks.resolve({ tasks: [task(1, 'Stale result')] })
      await initialTasks.promise
    })
    expect(result.current.tasks.map((entry) => entry.title)).toEqual(['New result'])
  })

  it('synchronizes list state when browser history changes the filter query', async () => {
    listTasks.mockImplementation(({ search }) => Promise.resolve({ tasks: [task(search ? 2 : 1, search ? 'History result' : 'Initial result')] }))
    resolveOptions()
    const navigate = vi.fn()
    const { result, rerender } = renderHook(({ query }) => useTaskDirectory(hookProps({ initialSearch: query, navigate })), { initialProps: { query: '' } })

    await waitFor(() => expect(result.current.tasks.map((entry) => entry.title)).toEqual(['Initial result']))
    rerender({ query: 'history' })

    await waitFor(() => expect(result.current.tasks.map((entry) => entry.title)).toEqual(['History result']))
    expect(result.current.search).toBe('history')
    expect(navigate).not.toHaveBeenCalled()
  })

  it('retains successful form options when one option request fails', async () => {
    listTasks.mockResolvedValue({ tasks: [] })
    listDeals.mockRejectedValue(new Error('Deals unavailable'))
    listCompanies.mockResolvedValue({ companies: [{ id: 4, name: 'Acme' }] })
    listContacts.mockResolvedValue({ contacts: [] })
    listOrganizationUsers.mockResolvedValue([])

    const { result } = renderHook(() => useTaskDirectory(hookProps()))

    await waitFor(() => expect(result.current.error).toBe('Deals unavailable'))
    expect(result.current.companyOptions).toEqual([{ id: 4, name: 'Acme' }])
    expect(result.current.tasks).toEqual([])
  })
})

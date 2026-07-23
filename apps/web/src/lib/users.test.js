import { afterEach, describe, expect, it, vi } from 'vitest'
import { listOrganizationUsers, listOrganizationUsersPage } from './users'

afterEach(() => vi.unstubAllGlobals())

describe('organization user API', () => {
  it('builds an exact searchable status page request', async () => {
    const controller = new AbortController()
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: { users: [{ id: 51 }], meta: { page: 2, pageSize: 25, total: 51 } } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listOrganizationUsersPage({
      search: 'Casey %_', status: 'disabled', page: 2, pageSize: 25, signal: controller.signal
    })).resolves.toEqual({ users: [{ id: 51 }], meta: { page: 2, pageSize: 25, total: 51 } })

    const requestURL = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(requestURL.pathname).toBe('/api/users')
    expect(Object.fromEntries(requestURL.searchParams)).toEqual({
      q: 'Casey %_', status: 'disabled', page: '2', pageSize: '25'
    })
    expect(fetchMock.mock.calls[0][1]).toEqual(expect.objectContaining({ signal: controller.signal }))
  })

  it('loads every bounded active page for teammate selectors', async () => {
    const users = Array.from({ length: 101 }, (_, index) => ({ id: index + 1, status: 'active' }))
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const page = Number(requestURL.searchParams.get('page') || 1)
      return {
        ok: true,
        json: async () => ({ data: { users: users.slice((page - 1) * 100, page * 100), meta: { page, pageSize: 100, total: users.length } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listOrganizationUsers()).resolves.toEqual(users)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    for (const call of fetchMock.mock.calls) {
      const requestURL = new URL(String(call[0]), 'http://localhost')
      expect(requestURL.searchParams.get('status')).toBe('active')
      expect(requestURL.searchParams.get('pageSize')).toBe('100')
    }
  })

  it('includes retained disabled history only when requested', async () => {
    const users = [{ id: 1, status: 'active' }, { id: 2, status: 'disabled' }]
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ data: { users, meta: { page: 1, pageSize: 100, total: 2 } } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listOrganizationUsers({ includeDisabled: true })).resolves.toEqual(users)
    const requestURL = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(requestURL.searchParams.get('status')).toBeNull()
    expect(requestURL.searchParams.get('pageSize')).toBe('100')
  })

  it('fails visibly on total drift or overlapping incomplete pages', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({ id: index + 1 }))
    const driftingFetch = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page') || 1)
      return {
        ok: true,
        json: async () => ({ data: { users: page === 1 ? firstPage : [], meta: { page, pageSize: 100, total: page === 1 ? 101 : 100 } } })
      }
    })
    vi.stubGlobal('fetch', driftingFetch)
    await expect(listOrganizationUsers()).rejects.toThrow('Team access changed while options loaded')

    const incompleteFetch = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page') || 1)
      const users = page === 1 ? firstPage : page === 2 ? [firstPage[99]] : []
      return { ok: true, json: async () => ({ data: { users, meta: { page, pageSize: 100, total: 101 } } }) }
    })
    vi.stubGlobal('fetch', incompleteFetch)
    await expect(listOrganizationUsers()).rejects.toThrow('Team access changed while options loaded')
  })
})

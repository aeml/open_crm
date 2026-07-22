import { afterEach, describe, expect, it, vi } from 'vitest'
import { deleteSavedView, listSavedViewPage, listSavedViews, updateSavedView } from './saved_views'

afterEach(() => vi.unstubAllGlobals())

function response(data) {
  return { ok: true, json: async () => ({ data }) }
}

describe('saved-view API', () => {
  it('builds a bounded personal management page query', async () => {
    const fetchMock = vi.fn(async () => response({ views: [{ id: 8 }], meta: { page: 2, pageSize: 25, total: 26 } }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(listSavedViewPage('contacts', { page: 2, pageSize: 25 })).resolves.toEqual({
      views: [{ id: 8 }], meta: { page: 2, pageSize: 25, total: 26 }
    })

    const requestURL = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(requestURL.pathname).toBe('/api/saved-views')
    expect(Object.fromEntries(requestURL.searchParams)).toEqual({ entityType: 'contacts', page: '2', pageSize: '25' })
  })

  it('preserves access to legacy overflow and deduplicates page rows', async () => {
    const rows = Array.from({ length: 101 }, (_, index) => ({ id: index + 1, name: `View ${index + 1}` }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      const pageRows = page === 1 ? rows.slice(0, 100) : [rows[99], rows[100]]
      return response({ views: pageRows, meta: { page, pageSize: 100, total: rows.length } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listSavedViews('contacts')).resolves.toEqual(rows)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('fails visibly when the catalog total changes between pages', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({ id: index + 1 }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return response({ views: page === 1 ? firstPage : [], meta: { page, pageSize: 100, total: page === 1 ? 101 : 100 } })
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listSavedViews('contacts')).rejects.toThrow('changed while options were loading')
  })

  it('binds edits and deletes to the reviewed revision', async () => {
    const fetchMock = vi.fn(async (_url, options) => options.method === 'DELETE'
      ? { ok: true, status: 204 }
      : response({ view: { id: 9, revision: 4 } }))
    vi.stubGlobal('fetch', fetchMock)

    await updateSavedView(9, { entityType: 'tasks', name: 'Reviewed', filters: {}, expectedRevision: 3 })
    await deleteSavedView(9, 4)

    const updateURL = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    const deleteURL = new URL(String(fetchMock.mock.calls[1][0]), 'http://localhost')
    expect(`${updateURL.pathname}${updateURL.search}`).toBe('/api/saved-views/9')
    expect(JSON.parse(fetchMock.mock.calls[0][1].body).expectedRevision).toBe(3)
    expect(`${deleteURL.pathname}${deleteURL.search}`).toBe('/api/saved-views/9?revision=4')
  })
})

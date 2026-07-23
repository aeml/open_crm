import { afterEach, describe, expect, it, vi } from 'vitest'
import { listLeadCaptureForms } from './lead_forms'

afterEach(() => vi.unstubAllGlobals())

describe('lead form API', () => {
  it('loads the complete bounded active catalog for dependent selectors', async () => {
    const forms = Array.from({ length: 101 }, (_, index) => ({ id: index + 1, name: `Lead form ${index + 1}`, isActive: true }))
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const page = Number(requestURL.searchParams.get('page'))
      return {
        ok: true,
        json: async () => ({ data: { forms: forms.slice((page - 1) * 100, page * 100), meta: { page, pageSize: 100, total: forms.length } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listLeadCaptureForms({ status: 'active' })).resolves.toEqual(forms)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    const firstRequest = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(firstRequest.pathname).toBe('/api/lead-capture-forms')
    expect(Object.fromEntries(firstRequest.searchParams)).toEqual({ status: 'active', page: '1', pageSize: '100' })
    expect(new URL(String(fetchMock.mock.calls[1][0]), 'http://localhost').searchParams.get('page')).toBe('2')
  })

  it('fails visibly when the catalog total changes between pages', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({ id: index + 1, name: `Lead form ${index + 1}` }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return {
        ok: true,
        json: async () => ({ data: { forms: page === 1 ? firstPage : [], meta: { page, pageSize: 100, total: page === 1 ? 101 : 100 } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listLeadCaptureForms()).rejects.toThrow('changed while options were loading')
  })

  it('fails visibly when pages overlap', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({ id: index + 1, name: `Lead form ${index + 1}` }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      const forms = page === 1 ? firstPage : page === 2 ? [firstPage[99]] : []
      return { ok: true, json: async () => ({ data: { forms, meta: { page, pageSize: 100, total: 101 } } }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listLeadCaptureForms()).rejects.toThrow('changed while options were loading')
  })
})

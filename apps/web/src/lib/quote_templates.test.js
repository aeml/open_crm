import { afterEach, describe, expect, it, vi } from 'vitest'
import { listQuoteTemplates } from './quote_templates'

afterEach(() => vi.unstubAllGlobals())

describe('quote template API', () => {
  it('loads only the bounded active quote-preparation catalog', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        data: {
          templates: [{ id: 7, name: 'Services terms', isActive: true }],
          meta: { page: 1, pageSize: 100, total: 1 }
        }
      })
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(listQuoteTemplates()).resolves.toEqual([{ id: 7, name: 'Services terms', isActive: true }])
    const requestURL = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(requestURL.pathname).toBe('/api/quote-templates')
    expect(Object.fromEntries(requestURL.searchParams)).toEqual({ status: 'active', page: '1', pageSize: '100' })
  })

  it('preserves quote access to legacy active templates above the current ceiling', async () => {
    const templates = Array.from({ length: 101 }, (_, index) => ({ id: index + 1, name: `Terms ${index + 1}`, isActive: true }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return {
        ok: true,
        json: async () => ({ data: { templates: templates.slice((page - 1) * 100, page * 100), meta: { page, pageSize: 100, total: templates.length } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listQuoteTemplates()).resolves.toEqual(templates)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(new URL(String(fetchMock.mock.calls[1][0]), 'http://localhost').searchParams.get('page')).toBe('2')
  })

  it('fails visibly when the active catalog changes between pages', async () => {
    const firstPage = Array.from({ length: 100 }, (_, index) => ({ id: index + 1, name: `Terms ${index + 1}`, isActive: true }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return {
        ok: true,
        json: async () => ({ data: { templates: page === 1 ? firstPage : [], meta: { page, pageSize: 100, total: page === 1 ? 101 : 100 } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listQuoteTemplates()).rejects.toThrow('changed while quote options were loading')
  })
})

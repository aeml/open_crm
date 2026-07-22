import { afterEach, describe, expect, it, vi } from 'vitest'
import { listProductCatalogItems } from './product_catalog'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('product catalog API', () => {
  it('loads only the bounded active quote-selection catalog', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({
        data: {
          items: [{ id: 7, name: 'Implementation', isActive: true }],
          meta: { page: 1, pageSize: 100, total: 1 }
        }
      })
    }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(listProductCatalogItems()).resolves.toEqual([{ id: 7, name: 'Implementation', isActive: true }])
    const requestURL = new URL(String(fetchMock.mock.calls[0][0]), 'http://localhost')
    expect(requestURL.pathname).toBe('/api/product-catalog-items')
    expect(Object.fromEntries(requestURL.searchParams)).toEqual({ status: 'active', page: '1', pageSize: '100' })
  })

  it('preserves quote access to a legacy active catalog above the current ceiling', async () => {
    const items = Array.from({ length: 101 }, (_, index) => ({ id: index + 1, name: `Service ${index + 1}`, isActive: true }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return {
        ok: true,
        json: async () => ({ data: { items: items.slice((page - 1) * 100, page * 100), meta: { page, pageSize: 100, total: items.length } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    await expect(listProductCatalogItems()).resolves.toEqual(items)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(new URL(String(fetchMock.mock.calls[1][0]), 'http://localhost').searchParams.get('page')).toBe('2')
  })
})

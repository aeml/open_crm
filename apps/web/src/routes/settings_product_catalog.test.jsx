import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function sessionResponse() {
  return {
    ok: true,
    json: async () => ({
      data: {
        user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
        organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
        membership: { role: 'owner' }
      }
    })
  }
}

describe('settings product catalog route', () => {
  it('lists catalog items and creates a new service', async () => {
    let catalogItems = [{ id: 3, name: 'Retainer', sku: 'RET-001', description: 'Monthly support', itemType: 'service', unitPrice: '2500.00', currency: 'USD', unitName: 'month', isActive: true }]
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/product-catalog-items') && method === 'POST') {
        const item = { id: 5, ...JSON.parse(options.body) }
        catalogItems = [item, ...catalogItems]
        return jsonResponse({ data: { item } })
      }
      if (path.endsWith('/api/product-catalog-items')) {
        return catalogListResponse(jsonResponse, requestURL, catalogItems)
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/product-catalog')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /product catalog/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /retainer/i })).toBeInTheDocument()
    expect(await screen.findByText(/RET-001/)).toBeInTheDocument()
    expect(catalogRequestURLs(fetchMock)[0].searchParams.get('page')).toBe('1')
    expect(catalogRequestURLs(fetchMock)[0].searchParams.get('pageSize')).toBe('50')

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Implementation' } })
    fireEvent.change(screen.getByLabelText(/^sku$/i), { target: { value: 'SERV-002' } })
    fireEvent.change(screen.getByLabelText(/^type$/i), { target: { value: 'service' } })
    fireEvent.change(screen.getByLabelText(/^unit price$/i), { target: { value: '150.00' } })
    fireEvent.change(screen.getByLabelText(/^currency$/i), { target: { value: 'USD' } })
    fireEvent.change(screen.getByLabelText(/^unit$/i), { target: { value: 'hour' } })
    fireEvent.change(screen.getByLabelText(/^description$/i), { target: { value: 'Project implementation' } })
    fireEvent.click(screen.getByRole('button', { name: /create catalog item/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/product-catalog-items') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Implementation',
        sku: 'SERV-002',
        description: 'Project implementation',
        itemType: 'service',
        unitPrice: '150.00',
        currency: 'USD',
        unitName: 'hour',
        isActive: true
      })
    })
    expect(await screen.findByRole('heading', { name: /implementation/i })).toBeInTheDocument()
  })

  it('continues, searches, and filters the bounded catalog', async () => {
    const catalogItems = Array.from({ length: 51 }, (_, index) => ({
      id: index + 1,
      name: `Catalog item ${String(index + 1).padStart(3, '0')}`,
      sku: `CAT-${String(index + 1).padStart(3, '0')}`,
      itemType: 'service',
      unitPrice: '25.00',
      currency: 'USD',
      unitName: 'hour',
      isActive: index === 0
    }))
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/auth/me')) return sessionResponse()
      if (requestURL.pathname.endsWith('/api/product-catalog-items')) return catalogListResponse(jsonResponse, requestURL, catalogItems)
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/product-catalog')
    render(<AppRouter />)

    expect(await screen.findByText(/Showing 50 of 51 catalog items/)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Catalog item 051' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    expect(await screen.findByRole('heading', { name: 'Catalog item 051' })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Search product catalog'), { target: { value: 'Catalog item 011' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply search' }))
    expect(await screen.findByRole('heading', { name: 'Catalog item 011' })).toBeInTheDocument()
    await waitFor(() => {
      const latest = catalogRequestURLs(fetchMock).at(-1)
      expect(latest.searchParams.get('q')).toBe('Catalog item 011')
      expect(latest.searchParams.get('page')).toBe('1')
    })

    fireEvent.change(screen.getByLabelText('Catalog status'), { target: { value: 'active' } })
    expect(await screen.findByText('No catalog items match these filters.')).toBeInTheDocument()
    await waitFor(() => expect(catalogRequestURLs(fetchMock).at(-1).searchParams.get('status')).toBe('active'))
  })
})

function catalogListResponse(jsonResponse, requestURL, catalogItems) {
  const search = (requestURL.searchParams.get('q') || '').toLowerCase()
  const status = requestURL.searchParams.get('status') || 'all'
  const page = Number(requestURL.searchParams.get('page') || 1)
  const pageSize = Number(requestURL.searchParams.get('pageSize') || 50)
  const filtered = catalogItems
    .filter((item) => status === 'all' || item.isActive === (status === 'active'))
    .filter((item) => !search || item.name.toLowerCase().includes(search) || item.sku.toLowerCase().includes(search))
    .sort((left, right) => Number(right.isActive) - Number(left.isActive) || left.name.localeCompare(right.name) || left.id - right.id)
  const offset = (page - 1) * pageSize
  return jsonResponse({ data: { items: filtered.slice(offset, offset + pageSize), meta: { page, pageSize, total: filtered.length } } })
}

function catalogRequestURLs(fetchMock) {
  return fetchMock.mock.calls
    .map((call) => new URL(String(call[0]), 'http://localhost'))
    .filter((url) => url.pathname.endsWith('/api/product-catalog-items'))
}

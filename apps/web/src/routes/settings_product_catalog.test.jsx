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
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/product-catalog-items') && method === 'POST') {
        return jsonResponse({ data: { item: { id: 5, name: 'Implementation', sku: 'SERV-002', description: 'Project implementation', itemType: 'service', unitPrice: '150.00', currency: 'USD', unitName: 'hour', isActive: true } } })
      }
      if (path.endsWith('/api/product-catalog-items')) {
        return jsonResponse({ data: { items: [{ id: 3, name: 'Retainer', sku: 'RET-001', description: 'Monthly support', itemType: 'service', unitPrice: '2500.00', currency: 'USD', unitName: 'month', isActive: true }] } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/product-catalog')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /product catalog/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /retainer/i })).toBeInTheDocument()
    expect(await screen.findByText(/RET-001/)).toBeInTheDocument()

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
})

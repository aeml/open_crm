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

describe('reports route', () => {
  it('lists report definitions and creates a revenue report definition', async () => {
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/report-definitions') && method === 'POST') {
        return jsonResponse({ data: { definition: { id: 8, name: 'Pipeline revenue by stage', description: '', sourceType: 'deals', columns: ['id', 'name', 'stageName', 'status'], filters: [{ field: 'status', operator: 'equals', value: 'open' }], groupBy: 'stageName', aggregation: { function: 'sum', field: 'valueAmount' }, isActive: true } } })
      }
      if (path.endsWith('/api/report-definitions')) {
        return jsonResponse({ data: { definitions: [{ id: 3, name: 'Contact source report', description: 'Contacts by lead source', sourceType: 'contacts', columns: ['firstName', 'lastName', 'email'], filters: [{ field: 'status', operator: 'equals', value: 'lead' }], groupBy: 'leadSource', aggregation: { function: 'count', field: '' }, isActive: true }] } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/reports')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^reports$/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /contact source report/i })).toBeInTheDocument()
    expect(screen.getByText(/contacts by lead source/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Pipeline revenue by stage' } })
    fireEvent.change(screen.getByLabelText(/^source object$/i), { target: { value: 'deals' } })
    fireEvent.click(screen.getByRole('button', { name: /add filter/i }))
    fireEvent.change(screen.getByLabelText(/^filter field 1$/i), { target: { value: 'status' } })
    fireEvent.change(screen.getByLabelText(/^filter value 1$/i), { target: { value: 'open' } })
    fireEvent.change(screen.getByLabelText(/^group by$/i), { target: { value: 'stageName' } })
    fireEvent.change(screen.getByLabelText(/^aggregation$/i), { target: { value: 'sum' } })
    fireEvent.change(screen.getByLabelText(/^aggregation field$/i), { target: { value: 'valueAmount' } })
    fireEvent.click(screen.getByRole('button', { name: /create report definition/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/report-definitions') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Pipeline revenue by stage',
        description: '',
        sourceType: 'deals',
        columns: ['id', 'name', 'stageName', 'status'],
        filters: [{ field: 'status', operator: 'equals', value: 'open' }],
        groupBy: 'stageName',
        aggregation: { function: 'sum', field: 'valueAmount' },
        isActive: true
      })
    })
    expect(await screen.findByRole('heading', { name: /pipeline revenue by stage/i })).toBeInTheDocument()
  })
})

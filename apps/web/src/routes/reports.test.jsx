import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
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
    let definitions = [{ id: 3, name: 'Contact source report', description: 'Contacts by lead source', sourceType: 'contacts', visualizationType: 'table', columns: ['firstName', 'lastName', 'email'], filters: [{ field: 'status', operator: 'equals', value: 'lead' }], groupBy: 'leadSource', aggregation: { function: 'count', field: '' }, isActive: true, updatedAt: '2026-07-21T17:00:00Z' }]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/report-definitions') && method === 'POST') {
        const definition = { id: 8, name: 'Pipeline revenue by stage', description: '', sourceType: 'deals', visualizationType: 'bar', visualizationContract: 'grouped_bar_v1', columns: [], filters: [{ field: 'status', operator: 'equals', value: 'open' }], groupBy: 'stageName', aggregation: { function: 'sum', field: 'valueAmount' }, isActive: true }
        definitions = [definition, ...definitions]
        return jsonResponse({ data: { definition } })
      }
      if (path.endsWith('/api/report-definitions/8/results')) {
        return jsonResponse({ data: { definitionId: 8, definitionName: 'Pipeline revenue by stage', sourceType: 'deals', visualizationType: 'bar', visualizationContract: 'grouped_bar_v1', columns: [{ key: 'stageName', label: 'Stage', dataType: 'text' }, { key: 'sumValueAmount', label: 'SUM Value amount', dataType: 'numeric' }], rows: [{ values: { stageName: 'Discovery', sumValueAmount: '25000.00' } }], page: 1, pageSize: 50, hasMore: false, generatedAt: '2026-07-21T18:00:00Z' } })
      }
      if (path.endsWith('/api/report-definitions/3/results')) {
        return jsonResponse({ data: { definitionId: 3, definitionName: 'Contact source report', sourceType: 'contacts', visualizationType: 'table', visualizationContract: '', columns: [{ key: 'leadSource', label: 'Lead source', dataType: 'text' }, { key: 'recordCount', label: 'Record count', dataType: 'integer' }], rows: [{ values: { leadSource: 'Referral', recordCount: '4' } }], page: 1, pageSize: 50, hasMore: false, generatedAt: '2026-07-21T18:00:00Z' } })
      }
      if (path.endsWith('/api/report-definitions')) {
        return jsonResponse({ data: { definitions, meta: { page: 1, pageSize: 50, total: definitions.length } } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/reports')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^reports$/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /contact source report/i })).toBeInTheDocument()
    expect(screen.getByText(/contacts by lead source/i)).toBeInTheDocument()

    const savedReport = screen.getByRole('heading', { name: /contact source report/i }).closest('article')
    const exportLink = within(savedReport).getByRole('link', { name: /download csv/i })
    expect(exportLink).toHaveAttribute('href', expect.stringMatching(/\/api\/report-definitions\/3\/export\.csv$/))
    fireEvent.click(within(savedReport).getByRole('button', { name: /run report/i }))
    const results = await screen.findByRole('region', { name: /contact source report results/i })
    expect(results).toHaveTextContent('Lead source')
    expect(results).toHaveTextContent('Referral')
    expect(results).toHaveTextContent('4')

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Pipeline revenue by stage' } })
    fireEvent.change(screen.getByLabelText(/^source object$/i), { target: { value: 'deals' } })
    fireEvent.change(screen.getByLabelText(/^visualization$/i), { target: { value: 'bar' } })
    fireEvent.click(screen.getByRole('button', { name: /add filter/i }))
    fireEvent.change(screen.getByLabelText(/^filter field 1$/i), { target: { value: 'status' } })
    fireEvent.change(screen.getByLabelText(/^filter value 1$/i), { target: { value: 'open' } })
    fireEvent.change(screen.getByLabelText(/^category \(group by\)$/i), { target: { value: 'stageName' } })
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
        visualizationType: 'bar',
        visualizationContract: 'grouped_bar_v1',
        columns: [],
        filters: [{ field: 'status', operator: 'equals', value: 'open' }],
        groupBy: 'stageName',
        aggregation: { function: 'sum', field: 'valueAmount' },
        isActive: true
      })
    })
    const barReportHeading = await screen.findByRole('heading', { name: /pipeline revenue by stage/i })
    const barReport = barReportHeading.closest('article')
    fireEvent.click(within(barReport).getByRole('button', { name: /run report/i }))
    expect(await within(barReport).findByRole('img', { name: /pipeline revenue by stage grouped bar chart/i })).toBeInTheDocument()
    expect(within(barReport).getByRole('region', { name: /pipeline revenue by stage chart data/i })).toHaveTextContent('Discovery')
    expect(within(barReport).getByRole('region', { name: /pipeline revenue by stage chart data/i })).toHaveTextContent('25000.00')
  })

  it('loads the 51st stored report definition from a bounded continuation page', async () => {
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const firstPage = Array.from({ length: 50 }, (_, index) => ({
      id: index + 1,
      name: `Saved report ${String(index + 1).padStart(2, '0')}`,
      sourceType: 'contacts',
      visualizationType: 'table',
      columns: ['email'],
      filters: [],
      groupBy: '',
      aggregation: { function: 'none', field: '' },
      isActive: true
    }))
    const row51 = { ...firstPage[0], id: 51, name: 'Saved report 51' }
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/report-definitions')) {
        const page = Number(requestURL.searchParams.get('page'))
        const definitions = page === 2 ? [row51] : firstPage
        return jsonResponse({ data: { definitions, meta: { page, pageSize: 50, total: 51 } } })
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/reports')

    render(<AppRouter />)

    expect(await screen.findByText('Showing 50 of 51 stored definitions.')).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: row51.name })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Load more stored report definitions' }))
    expect(await screen.findByRole('heading', { name: row51.name })).toBeInTheDocument()
    expect(screen.getByText('Showing 51 of 51 stored definitions.')).toBeInTheDocument()
    const continuationCall = fetchMock.mock.calls.find((call) => new URL(String(call[0]), 'http://localhost').searchParams.get('page') === '2')
    expect(new URL(String(continuationCall[0]), 'http://localhost').searchParams.get('pageSize')).toBe('50')
  })

  it('retains a successful create when first-page reconciliation fails', async () => {
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    let reportReads = 0
    const created = { id: 88, name: 'Recovered saved report', description: '', sourceType: 'contacts', visualizationType: 'table', visualizationContract: '', columns: ['firstName', 'lastName', 'email'], filters: [], groupBy: '', aggregation: { function: 'none', field: '' }, isActive: true }
    vi.stubGlobal('fetch', vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/report-definitions') && options.method === 'POST') return jsonResponse({ data: { definition: created } })
      if (path.endsWith('/api/report-definitions')) {
        reportReads += 1
        if (reportReads === 1) return jsonResponse({ data: { definitions: [], meta: { page: 1, pageSize: 50, total: 0 } } })
        return { ok: false, status: 503, json: async () => ({ error: { message: 'Report refresh unavailable.' } }) }
      }
      return jsonResponse({ data: { unreadCount: 0 } })
    }))
    window.history.pushState({}, '', '/reports')

    render(<AppRouter />)

    await screen.findByText('Showing 0 of 0 stored definitions.')
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: created.name } })
    fireEvent.click(screen.getByRole('button', { name: /create report definition/i }))

    expect(await screen.findByRole('heading', { name: created.name })).toBeInTheDocument()
    expect(screen.getByText('Showing 1 of 1 stored definitions.')).toBeInTheDocument()
    expect(screen.getByText('Report definition created. Reload the stored-definition list before another change.', { exact: true })).toHaveAttribute('role', 'status')
    expect(screen.getByText('Report refresh unavailable.')).toBeInTheDocument()
  })
})

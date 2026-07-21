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
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/report-definitions') && method === 'POST') {
        return jsonResponse({ data: { definition: { id: 8, name: 'Pipeline revenue by stage', description: '', sourceType: 'deals', visualizationType: 'bar', visualizationContract: 'grouped_bar_v1', columns: [], filters: [{ field: 'status', operator: 'equals', value: 'open' }], groupBy: 'stageName', aggregation: { function: 'sum', field: 'valueAmount' }, isActive: true } } })
      }
      if (path.endsWith('/api/report-definitions/8/results')) {
        return jsonResponse({ data: { definitionId: 8, definitionName: 'Pipeline revenue by stage', sourceType: 'deals', visualizationType: 'bar', visualizationContract: 'grouped_bar_v1', columns: [{ key: 'stageName', label: 'Stage', dataType: 'text' }, { key: 'sumValueAmount', label: 'SUM Value amount', dataType: 'numeric' }], rows: [{ values: { stageName: 'Discovery', sumValueAmount: '25000.00' } }], page: 1, pageSize: 50, hasMore: false, generatedAt: '2026-07-21T18:00:00Z' } })
      }
      if (path.endsWith('/api/report-definitions/3/results')) {
        return jsonResponse({ data: { definitionId: 3, definitionName: 'Contact source report', sourceType: 'contacts', visualizationType: 'table', visualizationContract: '', columns: [{ key: 'leadSource', label: 'Lead source', dataType: 'text' }, { key: 'recordCount', label: 'Record count', dataType: 'integer' }], rows: [{ values: { leadSource: 'Referral', recordCount: '4' } }], page: 1, pageSize: 50, hasMore: false, generatedAt: '2026-07-21T18:00:00Z' } })
      }
      if (path.endsWith('/api/report-definitions')) {
        return jsonResponse({ data: { definitions: [{ id: 3, name: 'Contact source report', description: 'Contacts by lead source', sourceType: 'contacts', visualizationType: 'table', columns: ['firstName', 'lastName', 'email'], filters: [{ field: 'status', operator: 'equals', value: 'lead' }], groupBy: 'leadSource', aggregation: { function: 'count', field: '' }, isActive: true, updatedAt: '2026-07-21T17:00:00Z' }] } })
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
})

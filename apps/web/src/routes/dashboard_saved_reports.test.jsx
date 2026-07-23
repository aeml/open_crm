import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { DashboardSavedReports } from './dashboard_saved_reports'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload, { ok = true, status = 200 } = {}) {
  return { ok, status, json: async () => payload }
}

function dashboardExecution() {
  const generatedAt = '2026-07-23T02:00:00Z'
  const definition = {
    id: 9,
    name: 'Pipeline by stage',
    description: 'Current deal count by stage.',
    sourceType: 'deals',
    visualizationType: 'bar',
    visualizationContract: 'grouped_bar_v1',
    columns: [],
    filters: [],
    groupBy: 'stageName',
    aggregation: { function: 'count', field: '' },
    isActive: true
  }
  return {
    revision: 3,
    generatedAt,
    widgets: [{
      position: 0,
      width: 'full',
      definition,
      result: {
        definitionId: 9,
        definitionName: definition.name,
        sourceType: 'deals',
        visualizationType: 'bar',
        visualizationContract: 'grouped_bar_v1',
        columns: [{ key: 'stageName', label: 'Stage', dataType: 'text' }, { key: 'recordCount', label: 'Record count', dataType: 'integer' }],
        rows: [{ values: { stageName: 'Discovery', recordCount: '4' } }, { values: { stageName: 'Proposal', recordCount: '2' } }],
        page: 1,
        pageSize: 12,
        hasMore: true,
        generatedAt
      }
    }]
  }
}

describe('dashboard saved reports', () => {
  it('renders one shared snapshot as a chart with its exact accessible table and truncation state', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ data: dashboardExecution() })))

    render(<MemoryRouter><DashboardSavedReports /></MemoryRouter>)

    expect(await screen.findByRole('heading', { name: 'Pipeline by stage' })).toBeInTheDocument()
    expect(screen.getByText(/all charts use one workspace snapshot generated/i)).toBeInTheDocument()
    expect(screen.getByRole('img', { name: /pipeline by stage grouped bar chart/i })).toBeInTheDocument()
    const tableRegion = screen.getByRole('region', { name: /pipeline by stage chart data/i })
    expect(tableRegion).toHaveTextContent('Discovery')
    expect(tableRegion).toHaveTextContent('4')
    expect(screen.getByText(/showing the first 12 categories/i)).toHaveAttribute('role', 'status')
    expect(screen.getByRole('button', { name: 'Refresh reports' })).toBeEnabled()
  })

  it('shows an actionable empty state for an unconfigured workspace', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => jsonResponse({ data: { revision: 0, generatedAt: '2026-07-23T02:00:00Z', widgets: [] } })))

    render(<MemoryRouter><DashboardSavedReports /></MemoryRouter>)

    expect(await screen.findByRole('heading', { name: 'No shared report charts yet' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open Reports' })).toBeInTheDocument()
  })

  it('recovers visibly from a stale dashboard configuration error', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(jsonResponse({ error: { code: 'DASHBOARD_CONFIGURATION_STALE', message: 'A dashboard report is no longer active and executable; update the shared dashboard' } }, { ok: false, status: 409 }))
      .mockResolvedValueOnce(jsonResponse({ data: { revision: 4, generatedAt: '2026-07-23T02:00:00Z', widgets: [] } }))
    vi.stubGlobal('fetch', fetchMock)

    render(<MemoryRouter><DashboardSavedReports /></MemoryRouter>)

    expect(await screen.findByText(/no longer active and executable/i)).toBeInTheDocument()
    expect(screen.getByText(/remove or replace it from Reports/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry shared dashboard' }))
    expect(await screen.findByRole('heading', { name: 'No shared report charts yet' })).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

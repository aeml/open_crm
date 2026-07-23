import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { ReportDashboardSettings } from './report_dashboard_settings'

afterEach(() => {
  vi.unstubAllGlobals()
})

function barDefinition(id, name, isActive = true) {
  return {
    id,
    name,
    description: '',
    sourceType: 'contacts',
    visualizationType: 'bar',
    visualizationContract: 'grouped_bar_v1',
    columns: [],
    filters: [],
    groupBy: 'status',
    aggregation: { function: 'count', field: '' },
    isActive
  }
}

function dashboardWidget(id, position, width, definition) {
  return { id: id + 100, reportDefinitionId: id, position, width, definition }
}

describe('shared report dashboard settings', () => {
  it('adds, removes, reorders, resizes, and saves an exact revision', async () => {
    const source = barDefinition(11, 'Source bars')
    const status = barDefinition(12, 'Status bars')
    const owner = barDefinition(13, 'Owner bars')
    const definitions = [source, status, owner, { ...barDefinition(14, 'Table report'), visualizationType: 'table', visualizationContract: '' }]
    const fetchMock = vi.fn(async (_url, options = {}) => {
      if (options.method === 'PUT') {
        const input = JSON.parse(options.body)
        return {
          ok: true,
          json: async () => ({
            data: {
              dashboard: {
                id: 5,
                revision: 3,
                widgets: input.widgets.map((widget, index) => dashboardWidget(widget.reportDefinitionId, index, widget.width, definitions.find((definition) => definition.id === widget.reportDefinitionId)))
              }
            }
          })
        }
      }
      return {
        ok: true,
        json: async () => ({ data: { dashboard: { id: 5, revision: 2, widgets: [dashboardWidget(11, 0, 'half', source), dashboardWidget(12, 1, 'full', status)] } } })
      }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<ReportDashboardSettings definitions={definitions} totalDefinitions={4} canManage />)

    const sourceRow = (await screen.findByText('Source bars')).closest('article')
    fireEvent.click(within(sourceRow).getByRole('button', { name: 'Move down' }))
    fireEvent.change(within(sourceRow).getByLabelText('Width for Source bars'), { target: { value: 'full' } })
    const statusRow = screen.getByText('Status bars').closest('article')
    fireEvent.click(within(statusRow).getByRole('button', { name: 'Remove' }))
    fireEvent.change(screen.getByLabelText('Add grouped-bar report'), { target: { value: '13' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add to dashboard' }))
    fireEvent.click(screen.getByRole('button', { name: 'Save dashboard' }))

    await waitFor(() => {
      const saveCall = fetchMock.mock.calls.find((call) => call[1]?.method === 'PUT')
      expect(saveCall).toBeTruthy()
      expect(JSON.parse(saveCall[1].body)).toEqual({
        revision: 2,
        widgets: [
          { reportDefinitionId: 11, width: 'full' },
          { reportDefinitionId: 13, width: 'half' }
        ]
      })
    })
    expect(await screen.findByText(/everyone in this workspace will see the same snapshot/i)).toHaveAttribute('role', 'status')
    expect(screen.getByRole('button', { name: 'Save dashboard' })).toBeDisabled()
    expect(screen.queryByText('Table report')).not.toBeInTheDocument()
  })

  it('keeps a stale selected report visible for removal and hides writer controls from viewers', async () => {
    const stale = barDefinition(21, 'Inactive shared bars', false)
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({ data: { dashboard: { id: 6, revision: 4, widgets: [dashboardWidget(21, 0, 'half', stale)] } } })
    })))

    render(<ReportDashboardSettings definitions={[]} totalDefinitions={51} canManage={false} />)

    expect(await screen.findByText('Inactive shared bars')).toBeInTheDocument()
    expect(screen.getByText(/remove or reactivate this report before saving/i)).toBeInTheDocument()
    expect(screen.getByText(/workspace writers manage this shared configuration/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Remove' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Save dashboard' })).not.toBeInTheDocument()
  })

  it('enforces the visible six-report ceiling and offers bounded definition continuation', async () => {
    const definitions = Array.from({ length: 6 }, (_, index) => barDefinition(index + 1, `Dashboard bar ${index + 1}`))
    const onLoadMore = vi.fn()
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({
        data: { dashboard: { id: 7, revision: 1, widgets: definitions.map((definition, index) => dashboardWidget(definition.id, index, 'half', definition)) } }
      })
    })))

    render(<ReportDashboardSettings definitions={definitions} totalDefinitions={51} canManage onLoadMore={onLoadMore} />)

    expect(await screen.findByText(/reached its six-report limit/i)).toHaveAttribute('role', 'status')
    expect(screen.getByLabelText('Add grouped-bar report')).toBeDisabled()
    fireEvent.click(screen.getByRole('button', { name: 'Load more definitions for dashboard' }))
    expect(onLoadMore).toHaveBeenCalledTimes(1)
  })
})

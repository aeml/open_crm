import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { CustomReportResults } from './custom_report_results'

const definition = {
  id: 7,
  name: 'Client follow-up',
  visualizationType: 'table',
  visualizationContract: '',
  sourceType: 'contacts',
  isActive: true,
  updatedAt: '2026-07-21T19:00:00Z'
}

afterEach(() => vi.unstubAllGlobals())

describe('custom report results', () => {
  it('shows the direct CSV download only to an authorized admin role', () => {
    const { rerender } = render(<CustomReportResults definition={definition} canExport={false} />)

    expect(screen.getByRole('button', { name: 'Run report' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Download CSV' })).not.toBeInTheDocument()

    rerender(<CustomReportResults definition={definition} canExport />)

    expect(screen.getByRole('link', { name: 'Download CSV' })).toHaveAttribute('href', expect.stringMatching(/\/api\/report-definitions\/7\/export\.csv$/))
  })

  it('refuses a stale or mismatched execution response', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({ data: { definitionId: 99, sourceType: 'contacts', visualizationType: 'table', visualizationContract: '', columns: [], rows: [], page: 1, pageSize: 50, hasMore: false } })
    })))
    render(<CustomReportResults definition={definition} />)
    fireEvent.click(screen.getByRole('button', { name: 'Run report' }))
    expect(await screen.findByText(/different definition/i)).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: /client follow-up results/i })).not.toBeInTheDocument()
  })
})

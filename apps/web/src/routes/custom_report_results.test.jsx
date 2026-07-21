import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CustomReportResults } from './custom_report_results'

const definition = {
  id: 7,
  name: 'Client follow-up',
  visualizationType: 'table',
  isActive: true,
  updatedAt: '2026-07-21T19:00:00Z'
}

describe('custom report results', () => {
  it('shows the direct CSV download only to an authorized admin role', () => {
    const { rerender } = render(<CustomReportResults definition={definition} canExport={false} />)

    expect(screen.getByRole('button', { name: 'Run report' })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Download CSV' })).not.toBeInTheDocument()

    rerender(<CustomReportResults definition={definition} canExport />)

    expect(screen.getByRole('link', { name: 'Download CSV' })).toHaveAttribute('href', expect.stringMatching(/\/api\/report-definitions\/7\/export\.csv$/))
  })
})

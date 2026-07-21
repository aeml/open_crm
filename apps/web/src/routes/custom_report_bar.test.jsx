import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { CustomReportBar } from './custom_report_bar'

const definition = { name: 'Pipeline value' }
const result = {
  page: 1,
  columns: [{ key: 'stage', label: 'Stage', dataType: 'text' }, { key: 'total', label: 'Total', dataType: 'numeric' }],
  rows: [{ values: { stage: 'Discovery', total: '25000.00' } }, { values: { stage: 'Proposal', total: '12500.00' } }]
}

describe('custom report grouped bar', () => {
  it('pairs the visual bars with an exact accessible data table', () => {
    render(<CustomReportBar definition={definition} result={result} />)
    expect(screen.getByRole('img', { name: /exact values follow in a data table/i })).toBeInTheDocument()
    const table = screen.getByRole('region', { name: /pipeline value chart data/i })
    expect(table).toHaveTextContent('Discovery')
    expect(table).toHaveTextContent('25000.00')
  })

  it('fails visibly when the aggregate is not numeric', () => {
    render(<CustomReportBar definition={definition} result={{ ...result, columns: [{ key: 'stage', label: 'Stage', dataType: 'text' }, { key: 'total', label: 'Total', dataType: 'text' }] }} />)
    expect(screen.getByRole('alert')).toHaveTextContent(/cannot be shown safely/i)
  })
})

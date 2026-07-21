import { describe, expect, it } from 'vitest'
import {
  aggregationFieldOptions,
  aggregationOptionsForVisualization,
  emptyForm,
  formFromDefinition,
  formWithVisualization,
  isExecutableReportDefinition,
  operatorsForField,
  payloadFromForm,
  reportSummary
} from './report_definition_model'

describe('report definition model', () => {
  it('keeps field operators and aggregation fields type-aware', () => {
    expect(operatorsForField('email').map(({ value }) => value)).toEqual(['equals', 'notEquals', 'contains', 'exists'])
    expect(operatorsForField('leadScore').map(({ value }) => value)).toEqual(['equals', 'notEquals', 'exists', 'greaterThan', 'lessThan'])
    expect(operatorsForField('createdAt').map(({ value }) => value)).toEqual(['equals', 'notEquals', 'exists', 'before', 'after'])
    expect(aggregationFieldOptions({ sourceType: 'deals', aggregationFunction: 'sum' }).map(({ value }) => value)).toEqual(['valueAmount'])
    expect(aggregationFieldOptions({ sourceType: 'contacts', aggregationFunction: 'none' })).toEqual([])
  })

  it('normalizes stored definitions and submission payloads without widening filters', () => {
    const form = formFromDefinition({
      name: 'Open pipeline',
      sourceType: 'deals',
      columns: ['name', 'status'],
      filters: [{ field: 'status', operator: 'equals', value: 'open' }],
      aggregation: { function: 'sum', field: 'valueAmount' },
      isActive: true
    })
    form.filters.push(
      { field: 'name', operator: 'contains', value: '   ' },
      { field: 'expectedCloseDate', operator: 'exists', value: '' }
    )

    expect(payloadFromForm(form)).toEqual({
      name: 'Open pipeline',
      description: '',
      sourceType: 'deals',
      visualizationType: 'table',
      visualizationContract: '',
      columns: ['name', 'status'],
      filters: [
        { field: 'status', operator: 'equals', value: 'open' },
        { field: 'expectedCloseDate', operator: 'exists', value: '' }
      ],
      groupBy: '',
      aggregation: { function: 'sum', field: 'valueAmount' },
      isActive: true
    })
    expect(reportSummary(payloadFromForm(form))).toContain('Table | Deals | 2 fields | 2 filters | no grouping | sum Value amount')
  })

  it('starts each source with bounded default columns', () => {
    expect(emptyForm('contacts').columns).toEqual(['id', 'firstName', 'lastName', 'email'])
    expect(emptyForm('tasks').columns).toEqual(['id', 'title', 'status', 'entityType'])
  })

  it('builds only grouped numeric bar definitions and restores table fields', () => {
    const bar = formWithVisualization({ ...emptyForm('deals'), aggregationFunction: 'sum' }, 'bar')
    expect(bar).toMatchObject({ visualizationType: 'bar', visualizationContract: 'grouped_bar_v1', columns: [], groupBy: 'status', aggregationFunction: 'sum', aggregationField: 'valueAmount' })
    expect(aggregationOptionsForVisualization('bar', 'deals').map(({ value }) => value)).toEqual(['count', 'sum', 'avg'])
    expect(aggregationOptionsForVisualization('bar', 'companies').map(({ value }) => value)).toEqual(['count'])
    expect(payloadFromForm(bar)).toMatchObject({ visualizationType: 'bar', visualizationContract: 'grouped_bar_v1', columns: [], groupBy: 'status', aggregation: { function: 'sum', field: 'valueAmount' } })
    expect(reportSummary(payloadFromForm(bar))).toContain('Bar chart | Deals | 0 filters | grouped by Status | sum Value amount')
    expect(formWithVisualization(bar, 'table').columns).toEqual(['id', 'name', 'stageName', 'status'])
    expect(isExecutableReportDefinition({ visualizationType: 'bar', visualizationContract: '' })).toBe(false)
    expect(isExecutableReportDefinition(payloadFromForm(bar))).toBe(true)
  })
})

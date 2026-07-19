import { describe, expect, it } from 'vitest'
import { appendCustomFieldParams, customFieldFilterFromParams, customFieldFormValues, customFieldOperators, customFieldPayload } from './custom_fields'

const definitions = [
  { fieldKey: 'region', dataType: 'text' },
  { fieldKey: 'annual_value', dataType: 'number' },
  { fieldKey: 'renewing', dataType: 'boolean' }
]

describe('custom field helpers', () => {
  it('round-trips typed form values without losing false or zero', () => {
    const form = customFieldFormValues(definitions, { region: 'West', annual_value: 0, renewing: false })
    expect(form).toEqual({ region: 'West', annual_value: '0', renewing: 'false' })
    expect(customFieldPayload(definitions, form)).toEqual({ region: 'West', annual_value: 0, renewing: false })
  })

  it('uses only valid operators for each field type', () => {
    expect(customFieldOperators('text').map((item) => item.value)).toEqual(['contains', 'eq'])
    expect(customFieldOperators('number').map((item) => item.value)).toEqual(['eq', 'gte', 'lte'])
    expect(customFieldOperators('date').map((item) => item.value)).toEqual(['eq', 'before', 'after'])
    expect(customFieldOperators('boolean').map((item) => item.value)).toEqual(['eq'])
  })

  it('round-trips filter URL parameters', () => {
    const params = appendCustomFieldParams(new URLSearchParams(), { fieldKey: 'region', operator: 'contains', value: 'North' })
    expect(params.toString()).toBe('customField=region&customOperator=contains&customValue=North')
    expect(customFieldFilterFromParams(params)).toEqual({ fieldKey: 'region', operator: 'contains', value: 'North' })
  })
})

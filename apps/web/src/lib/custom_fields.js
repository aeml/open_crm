import { apiRequest } from './api'

export async function listCustomFields(entityType, { signal } = {}) {
  const catalog = await listCustomFieldCatalog(entityType, { signal })
  return catalog.definitions
}

export async function listCustomFieldCatalog(entityType, { signal } = {}) {
  const params = new URLSearchParams({ entityType })
  const payload = await apiRequest(`/api/custom-fields?${params.toString()}`, { fallbackMessage: 'Unable to load custom fields.', signal })
  const definitions = payload?.data?.definitions || []
  return {
    definitions,
    total: Number(payload?.meta?.total ?? definitions.length),
    limit: Number(payload?.meta?.limit ?? 25)
  }
}

export async function createCustomField(input) {
  const payload = await apiRequest('/api/custom-fields', { method: 'POST', body: input, fallbackMessage: 'Unable to create custom field.' })
  return payload?.data?.definition || null
}

export async function updateCustomField(definitionId, input) {
  const payload = await apiRequest(`/api/custom-fields/${definitionId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update custom field.' })
  return payload?.data?.definition || null
}

export async function archiveCustomField(definitionId, revision) {
  return apiRequest(`/api/custom-fields/${definitionId}?revision=${encodeURIComponent(revision)}`, { method: 'DELETE', fallbackMessage: 'Unable to archive custom field.' })
}

export function customFieldFormValues(definitions, values = {}) {
  return Object.fromEntries((definitions || []).map((definition) => {
    const value = values?.[definition.fieldKey]
    if (value === undefined || value === null) return [definition.fieldKey, '']
    if (definition.dataType === 'boolean') return [definition.fieldKey, value ? 'true' : 'false']
    return [definition.fieldKey, String(value)]
  }))
}

export function customFieldPayload(definitions, values = {}) {
  return Object.fromEntries((definitions || []).map((definition) => {
    const value = values?.[definition.fieldKey]
    if (value === '' || value === undefined || value === null) return [definition.fieldKey, null]
    if (definition.dataType === 'number') return [definition.fieldKey, Number(value)]
    if (definition.dataType === 'boolean') return [definition.fieldKey, value === true || value === 'true']
    return [definition.fieldKey, String(value)]
  }))
}

export function customFieldOperators(dataType) {
  if (dataType === 'text') return [{ value: 'contains', label: 'contains' }, { value: 'eq', label: 'equals' }]
  if (dataType === 'number') return [{ value: 'eq', label: 'equals' }, { value: 'gte', label: 'at least' }, { value: 'lte', label: 'at most' }]
  if (dataType === 'date') return [{ value: 'eq', label: 'is' }, { value: 'before', label: 'before' }, { value: 'after', label: 'after' }]
  return [{ value: 'eq', label: 'is' }]
}

export function appendCustomFieldParams(params, filter = {}) {
  if (!filter.fieldKey) return params
  params.set('customField', filter.fieldKey)
  params.set('customOperator', filter.operator || 'eq')
  params.set('customValue', filter.value ?? '')
  return params
}

export function customFieldFilterFromParams(params) {
  return {
    fieldKey: params.get('customField') || '',
    operator: params.get('customOperator') || '',
    value: params.get('customValue') || ''
  }
}

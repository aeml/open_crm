export const sourceOptions = [
  { value: 'contacts', label: 'Contacts' },
  { value: 'companies', label: 'Companies' },
  { value: 'deals', label: 'Deals' },
  { value: 'tasks', label: 'Tasks' }
]

const productionVisualizationOptions = [
  { value: 'table', label: 'Table' },
  { value: 'bar', label: 'Bar chart' }
]

const developmentVisualizationOptions = [
  ...productionVisualizationOptions,
  { value: 'line', label: 'Line chart' },
  { value: 'funnel', label: 'Funnel' },
  { value: 'pie', label: 'Pie chart' },
  { value: 'kpi', label: 'KPI card' }
]

export const visualizationOptions = import.meta.env.DEV
  ? developmentVisualizationOptions
  : productionVisualizationOptions

const fieldOptionsBySource = {
  contacts: [
    { value: 'id', label: 'Contact ID' },
    { value: 'firstName', label: 'First name' },
    { value: 'lastName', label: 'Last name' },
    { value: 'email', label: 'Email' },
    { value: 'phone', label: 'Phone' },
    { value: 'status', label: 'Status' },
    { value: 'ownerUserId', label: 'Owner' },
    { value: 'leadSource', label: 'Lead source' },
    { value: 'leadScore', label: 'Lead score' },
    { value: 'createdAt', label: 'Created at' },
    { value: 'updatedAt', label: 'Updated at' }
  ],
  companies: [
    { value: 'id', label: 'Company ID' },
    { value: 'name', label: 'Name' },
    { value: 'clientType', label: 'Client type' },
    { value: 'industry', label: 'Industry' },
    { value: 'status', label: 'Status' },
    { value: 'city', label: 'City' },
    { value: 'state', label: 'State' },
    { value: 'country', label: 'Country' },
    { value: 'createdAt', label: 'Created at' },
    { value: 'updatedAt', label: 'Updated at' }
  ],
  deals: [
    { value: 'id', label: 'Deal ID' },
    { value: 'name', label: 'Deal name' },
    { value: 'stageName', label: 'Stage' },
    { value: 'status', label: 'Status' },
    { value: 'valueAmount', label: 'Value amount' },
    { value: 'valueCurrency', label: 'Currency' },
    { value: 'ownerUserId', label: 'Owner' },
    { value: 'expectedCloseDate', label: 'Expected close' },
    { value: 'createdAt', label: 'Created at' },
    { value: 'updatedAt', label: 'Updated at' }
  ],
  tasks: [
    { value: 'id', label: 'Task ID' },
    { value: 'title', label: 'Title' },
    { value: 'status', label: 'Status' },
    { value: 'entityType', label: 'Related record type' },
    { value: 'assignedToUserId', label: 'Assignee' },
    { value: 'dueAt', label: 'Due at' },
    { value: 'completedAt', label: 'Completed at' },
    { value: 'createdAt', label: 'Created at' },
    { value: 'updatedAt', label: 'Updated at' }
  ]
}

const numericFieldsBySource = {
  contacts: ['leadScore'],
  deals: ['valueAmount']
}

export const groupedBarVisualizationContract = 'grouped_bar_v1'

export function isExecutableReportDefinition(definition) {
  return definition.visualizationType === 'table' || (definition.visualizationType === 'bar' && definition.visualizationContract === groupedBarVisualizationContract)
}

const operatorOptions = [
  { value: 'equals', label: 'equals' },
  { value: 'notEquals', label: 'does not equal' },
  { value: 'contains', label: 'contains' },
  { value: 'exists', label: 'exists' },
  { value: 'greaterThan', label: 'greater than' },
  { value: 'lessThan', label: 'less than' },
  { value: 'before', label: 'before' },
  { value: 'after', label: 'after' }
]

const comparableFields = new Set(['id', 'ownerUserId', 'assignedToUserId', 'leadScore', 'valueAmount'])
export const temporalFields = new Set(['expectedCloseDate', 'dueAt', 'completedAt', 'createdAt', 'updatedAt'])

export function operatorsForField(field) {
  if (comparableFields.has(field)) {
    return operatorOptions.filter((option) => ['equals', 'notEquals', 'greaterThan', 'lessThan', 'exists'].includes(option.value))
  }
  if (temporalFields.has(field)) {
    return operatorOptions.filter((option) => ['equals', 'notEquals', 'before', 'after', 'exists'].includes(option.value))
  }
  return operatorOptions.filter((option) => ['equals', 'notEquals', 'contains', 'exists'].includes(option.value))
}

export const aggregationOptions = [
  { value: 'count', label: 'Count records' },
  { value: 'sum', label: 'Sum a numeric field' },
  { value: 'avg', label: 'Average a numeric field' },
  { value: 'min', label: 'Minimum field value' },
  { value: 'max', label: 'Maximum field value' },
  { value: 'none', label: 'No aggregation' }
]

export function aggregationOptionsForVisualization(visualizationType, sourceType) {
  if (visualizationType !== 'bar') return aggregationOptions
  const functions = numericFieldsBySource[sourceType]?.length ? ['count', 'sum', 'avg'] : ['count']
  return aggregationOptions.filter((option) => functions.includes(option.value))
}

export function fieldsForSource(sourceType) {
  return fieldOptionsBySource[sourceType] || fieldOptionsBySource.contacts
}

function fieldLabel(sourceType, field) {
  return fieldsForSource(sourceType).find((option) => option.value === field)?.label || field || 'None'
}

export function sourceLabel(sourceType) {
  return sourceOptions.find((option) => option.value === sourceType)?.label || sourceType
}

export function visualizationLabel(visualizationType) {
  return visualizationOptions.find((option) => option.value === visualizationType)?.label || visualizationType || 'Table'
}

export function defaultColumns(sourceType) {
  return fieldsForSource(sourceType).slice(0, 4).map((option) => option.value)
}

export function emptyFilter(sourceType) {
  return { field: fieldsForSource(sourceType)[0]?.value || '', operator: 'equals', value: '' }
}

export function emptyForm(sourceType = 'contacts') {
  return {
    name: '',
    description: '',
    sourceType,
    visualizationType: 'table',
    visualizationContract: '',
    columns: defaultColumns(sourceType),
    filters: [],
    groupBy: '',
    aggregationFunction: 'none',
    aggregationField: '',
    isActive: true
  }
}

function defaultGroupBy(sourceType) {
  const fields = fieldsForSource(sourceType)
  return fields.find((field) => field.value === 'status')?.value || fields.find((field) => field.value !== 'id')?.value || fields[0]?.value || ''
}

export function formWithVisualization(form, visualizationType) {
  if (visualizationType === 'bar') {
    const allowedFunctions = aggregationOptionsForVisualization('bar', form.sourceType).map((option) => option.value)
    const aggregationFunction = allowedFunctions.includes(form.aggregationFunction) ? form.aggregationFunction : 'count'
    const next = { ...form, visualizationType, visualizationContract: groupedBarVisualizationContract, columns: [], groupBy: form.groupBy || defaultGroupBy(form.sourceType), aggregationFunction }
    return { ...next, aggregationField: aggregationFieldOptions(next)[0]?.value || '' }
  }
  return { ...form, visualizationType, visualizationContract: '', columns: form.columns.length ? form.columns : defaultColumns(form.sourceType) }
}

export function formFromDefinition(definition) {
  const sourceType = definition.sourceType || 'contacts'
  const form = {
    name: definition.name || '',
    description: definition.description || '',
    sourceType,
    visualizationType: definition.visualizationType || 'table',
    visualizationContract: definition.visualizationContract || '',
    columns: definition.columns?.length ? definition.columns : defaultColumns(sourceType),
    filters: definition.filters || [],
    groupBy: definition.groupBy || '',
    aggregationFunction: definition.aggregation?.function || 'count',
    aggregationField: definition.aggregation?.field || '',
    isActive: definition.isActive !== false
  }
  return definition.visualizationType === 'bar' ? formWithVisualization(form, 'bar') : form
}

export function aggregationFieldOptions(form) {
  if (form.aggregationFunction === 'sum' || form.aggregationFunction === 'avg') {
    const numericFields = numericFieldsBySource[form.sourceType] || []
    return fieldsForSource(form.sourceType).filter((option) => numericFields.includes(option.value))
  }
  if (form.aggregationFunction === 'min' || form.aggregationFunction === 'max') {
    return fieldsForSource(form.sourceType)
  }
  return []
}

function aggregationSummary(definition) {
  const aggregation = definition.aggregation || { function: 'count' }
  if (aggregation.function === 'none') return 'No aggregation'
  if (aggregation.function === 'count') return 'Count records'
  return `${aggregation.function || 'count'} ${fieldLabel(definition.sourceType, aggregation.field)}`
}

export function payloadFromForm(form) {
  const filters = form.filters
    .map((filter) => ({ field: filter.field, operator: filter.operator, value: String(filter.value || '').trim() }))
    .filter((filter) => filter.field && filter.operator && (filter.operator === 'exists' || filter.value))

  return {
    name: form.name,
    description: form.description,
    sourceType: form.sourceType,
    visualizationType: form.visualizationType,
    visualizationContract: form.visualizationType === 'bar' ? groupedBarVisualizationContract : '',
    columns: form.visualizationType === 'bar' ? [] : form.columns,
    filters,
    groupBy: form.groupBy,
    aggregation: {
      function: form.aggregationFunction,
      field: form.aggregationField
    },
    isActive: form.isActive
  }
}

export function reportSummary(definition) {
  const columns = definition.columns || []
  const filters = definition.filters || []
  const group = definition.groupBy ? `grouped by ${fieldLabel(definition.sourceType, definition.groupBy)}` : 'no grouping'
  const output = definition.visualizationType === 'bar' ? '' : ` | ${columns.length} field${columns.length === 1 ? '' : 's'}`
  return `${visualizationLabel(definition.visualizationType || 'table')} | ${sourceLabel(definition.sourceType)}${output} | ${filters.length} filter${filters.length === 1 ? '' : 's'} | ${group} | ${aggregationSummary(definition)}`
}

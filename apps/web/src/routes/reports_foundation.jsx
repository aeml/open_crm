import { useEffect, useState } from 'react'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { createReportDefinition, listReportDefinitions, updateReportDefinition } from '../lib/reports'
import { usePageTitle } from '../lib/use_page_title'
import { DataQualityPanel } from './data_quality_panel'
import { SalesActivityReport } from './sales_activity_report'
import { FollowUpReport } from './follow_up_report'
import { CustomReportResults } from './custom_report_results'

const sourceOptions = [
  { value: 'contacts', label: 'Contacts' },
  { value: 'companies', label: 'Companies' },
  { value: 'deals', label: 'Deals' },
  { value: 'tasks', label: 'Tasks' }
]

const allVisualizationOptions = [
  { value: 'table', label: 'Table' },
  { value: 'bar', label: 'Bar chart' },
  { value: 'line', label: 'Line chart' },
  { value: 'funnel', label: 'Funnel' },
  { value: 'pie', label: 'Pie chart' },
  { value: 'kpi', label: 'KPI card' }
]

const visualizationOptions = import.meta.env.DEV
  ? allVisualizationOptions
  : allVisualizationOptions.filter((option) => option.value === 'table')

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
const temporalFields = new Set(['expectedCloseDate', 'dueAt', 'completedAt', 'createdAt', 'updatedAt'])

function operatorsForField(field) {
  if (comparableFields.has(field)) {
    return operatorOptions.filter((option) => ['equals', 'notEquals', 'greaterThan', 'lessThan', 'exists'].includes(option.value))
  }
  if (temporalFields.has(field)) {
    return operatorOptions.filter((option) => ['equals', 'notEquals', 'before', 'after', 'exists'].includes(option.value))
  }
  return operatorOptions.filter((option) => ['equals', 'notEquals', 'contains', 'exists'].includes(option.value))
}

const aggregationOptions = [
  { value: 'count', label: 'Count records' },
  { value: 'sum', label: 'Sum a numeric field' },
  { value: 'avg', label: 'Average a numeric field' },
  { value: 'min', label: 'Minimum field value' },
  { value: 'max', label: 'Maximum field value' },
  { value: 'none', label: 'No aggregation' }
]

function fieldsForSource(sourceType) {
  return fieldOptionsBySource[sourceType] || fieldOptionsBySource.contacts
}

function fieldLabel(sourceType, field) {
  return fieldsForSource(sourceType).find((option) => option.value === field)?.label || field || 'None'
}

function sourceLabel(sourceType) {
  return sourceOptions.find((option) => option.value === sourceType)?.label || sourceType
}

function visualizationLabel(visualizationType) {
  return visualizationOptions.find((option) => option.value === visualizationType)?.label || visualizationType || 'Table'
}

function defaultColumns(sourceType) {
  return fieldsForSource(sourceType).slice(0, 4).map((option) => option.value)
}

function emptyFilter(sourceType) {
  return { field: fieldsForSource(sourceType)[0]?.value || '', operator: 'equals', value: '' }
}

function emptyForm(sourceType = 'contacts') {
  return {
    name: '',
    description: '',
    sourceType,
    visualizationType: 'table',
    columns: defaultColumns(sourceType),
    filters: [],
    groupBy: '',
    aggregationFunction: 'none',
    aggregationField: '',
    isActive: true
  }
}

function formFromDefinition(definition) {
  const sourceType = definition.sourceType || 'contacts'
  return {
    name: definition.name || '',
    description: definition.description || '',
    sourceType,
    visualizationType: definition.visualizationType || 'table',
    columns: definition.columns?.length ? definition.columns : defaultColumns(sourceType),
    filters: definition.filters || [],
    groupBy: definition.groupBy || '',
    aggregationFunction: definition.aggregation?.function || 'count',
    aggregationField: definition.aggregation?.field || '',
    isActive: definition.isActive !== false
  }
}

function aggregationFieldOptions(form) {
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

function payloadFromForm(form) {
  const filters = form.filters
    .map((filter) => ({ field: filter.field, operator: filter.operator, value: String(filter.value || '').trim() }))
    .filter((filter) => filter.field && filter.operator && (filter.operator === 'exists' || filter.value))

  return {
    name: form.name,
    description: form.description,
    sourceType: form.sourceType,
    visualizationType: form.visualizationType,
    columns: form.columns,
    filters,
    groupBy: form.groupBy,
    aggregation: {
      function: form.aggregationFunction,
      field: form.aggregationField
    },
    isActive: form.isActive
  }
}

function reportSummary(definition) {
  const columns = definition.columns || []
  const filters = definition.filters || []
  const group = definition.groupBy ? `grouped by ${fieldLabel(definition.sourceType, definition.groupBy)}` : 'no grouping'
  return `${visualizationLabel(definition.visualizationType || 'table')} | ${sourceLabel(definition.sourceType)} | ${columns.length} field${columns.length === 1 ? '' : 's'} | ${filters.length} filter${filters.length === 1 ? '' : 's'} | ${group} | ${aggregationSummary(definition)}`
}

export function ReportsFoundationRoute() {
  const { session, canWrite: canManage } = useAuth()
  usePageTitle('Reports')
  const [definitions, setDefinitions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [editingId, setEditingId] = useState(null)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [isSaving, setIsSaving] = useState(false)

  async function loadDefinitions({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextDefinitions = await listReportDefinitions({ signal })
      setDefinitions(nextDefinitions)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load report definitions.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    loadDefinitions({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  function resetForm() {
    setEditingId(null)
    setForm(emptyForm())
  }

  function startEdit(definition) {
    setEditingId(definition.id)
    setForm(formFromDefinition(definition))
    setStatus('')
  }

  function setSourceType(sourceType) {
    setForm((current) => ({
      ...current,
      sourceType,
      visualizationType: 'table',
      columns: defaultColumns(sourceType),
      filters: [],
      groupBy: '',
      aggregationFunction: 'count',
      aggregationField: ''
    }))
  }

  function toggleColumn(column) {
    setForm((current) => ({
      ...current,
      columns: current.columns.includes(column)
        ? current.columns.filter((item) => item !== column)
        : [...current.columns, column]
    }))
  }

  function addFilter() {
    setForm((current) => ({ ...current, filters: [...current.filters, emptyFilter(current.sourceType)] }))
  }

  function updateFilter(index, patch) {
    setForm((current) => ({
      ...current,
      filters: current.filters.map((filter, filterIndex) => (filterIndex === index ? { ...filter, ...patch } : filter))
    }))
  }

  function removeFilter(index) {
    setForm((current) => ({ ...current, filters: current.filters.filter((_, filterIndex) => filterIndex !== index) }))
  }

  function setAggregationFunction(functionName) {
    setForm((current) => ({
      ...current,
      aggregationFunction: functionName,
      aggregationField: aggregationFieldOptions({ ...current, aggregationFunction: functionName })[0]?.value || '',
      groupBy: functionName === 'none' ? '' : current.groupBy
    }))
  }

  async function handleSubmit(event) {
    event.preventDefault()
    setIsSaving(true)
    setStatus('')
    try {
      const payload = payloadFromForm(form)
      if (editingId) {
        const updated = await updateReportDefinition(editingId, payload)
        setDefinitions((current) => current.map((definition) => (definition.id === editingId ? updated : definition)))
        setStatus('Report definition updated.')
      } else {
        const created = await createReportDefinition(payload)
        setDefinitions((current) => [created, ...current])
        setStatus('Report definition created.')
      }
      resetForm()
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to save report definition.')
    } finally {
      setIsSaving(false)
    }
  }

  const aggregationFields = aggregationFieldOptions(form)
  const visibleDefinitions = import.meta.env.DEV
    ? definitions
    : definitions.filter((definition) => definition.visualizationType === 'table')

  return (
    <section className="dashboard-grid settings-grid">
      <SalesActivityReport />
      <FollowUpReport />
      <DataQualityPanel />
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Reports</h2>
              <p>Build and run bounded saved table reports for {session?.organization?.name || 'your team'}. Chart, dashboard, sharing, export, and scheduling foundations remain hidden until their runtime outcomes are complete.</p>
            </div>
          </div>
          {isLoading ? <p className="field-hint">Loading report definitions...</p> : null}
          {status ? <p className="field-hint" role="status">{status}</p> : null}
          {error ? <InlineError message={error} onRetry={() => loadDefinitions()} retryLabel="Retry reports" /> : null}
          <div className="record-list" role="list" aria-label="Report definitions">
            {!isLoading && visibleDefinitions.length === 0 ? (
              <article className="record-row" role="listitem">
                <div>
                  <p>No report definitions yet.</p>
                  <p className="field-hint">Create a table report to query current workspace records with bounded, tenant-safe filters and pagination.</p>
                </div>
              </article>
            ) : visibleDefinitions.map((definition) => (
              <article className={definition.isActive ? 'record-row custom-report-row' : 'record-row record-row-alert custom-report-row'} key={definition.id} role="listitem">
                <div>
                  <h3>{definition.name}</h3>
                  <p className="field-hint">{reportSummary(definition)}</p>
                  {definition.description ? <p className="field-hint">{definition.description}</p> : null}
                </div>
                <div>
                  <span className="chip">{definition.isActive ? 'Active' : 'Inactive'}</span>
                  <span className="chip">{visualizationLabel(definition.visualizationType || 'table')}</span>
                  <span className="chip">{definition.visualizationType === 'table' ? 'Executable table' : 'Definition only'}</span>
                  {canManage && (import.meta.env.DEV || definition.visualizationType === 'table') ? <Button className="button-secondary" type="button" onClick={() => startEdit(definition)}>Edit</Button> : null}
                </div>
                <CustomReportResults definition={definition} />
              </article>
            ))}
          </div>
        </div>
      </Card>

      {canManage ? (
        <Card>
          <form className="auth-form card-stack" onSubmit={handleSubmit}>
            <div>
              <h2>{editingId ? 'Edit report definition' : 'New report definition'}</h2>
              <p className="field-hint">Saved table reports execute immediately with tenant-safe fields, typed filters, optional grouping, and bounded pagination. Other visualization types remain development-only.</p>
            </div>
            <Field label="Name">
              <input className="text-input" value={form.name} onChange={(event) => setForm({ ...form, name: event.target.value })} placeholder="Pipeline revenue by stage" required />
            </Field>
            <Field label="Description">
              <textarea className="text-input" rows={3} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} placeholder="Tracks open pipeline value by stage." />
            </Field>
            <Field label="Source object">
              <select className="text-input" value={form.sourceType} onChange={(event) => setSourceType(event.target.value)}>
                {sourceOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Visualization">
              <select className="text-input" value={form.visualizationType} onChange={(event) => setForm({ ...form, visualizationType: event.target.value })}>
                {visualizationOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>

            <div className="card-stack">
              <div>
                <h3>Fields</h3>
                <p className="field-hint">Choose at least one field to include in the report output.</p>
              </div>
              <div className="record-list" role="group" aria-label="Report fields">
                {fieldsForSource(form.sourceType).map((field) => (
                  <label className="field-hint" key={field.value}>
                    <input type="checkbox" checked={form.columns.includes(field.value)} onChange={() => toggleColumn(field.value)} /> {field.label}
                  </label>
                ))}
              </div>
            </div>

            <div className="card-stack">
              <div className="section-header">
                <div>
                  <h3>Filters</h3>
                  <p className="field-hint">Filters are validated by field type and applied when the report runs.</p>
                </div>
                <Button className="button-secondary" type="button" onClick={addFilter}>Add filter</Button>
              </div>
              {form.filters.length === 0 ? <p className="field-hint">No filters. The report definition includes all {sourceLabel(form.sourceType).toLowerCase()}.</p> : null}
              {form.filters.map((filter, index) => (
                <div className="record-row" key={`${filter.field}-${index}`}>
                  <Field label={`Filter field ${index + 1}`}>
                    <select className="text-input" value={filter.field} onChange={(event) => updateFilter(index, { field: event.target.value, operator: operatorsForField(event.target.value)[0].value, value: '' })}>
                      {fieldsForSource(form.sourceType).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                    </select>
                  </Field>
                  <Field label={`Filter operator ${index + 1}`}>
                    <select className="text-input" value={filter.operator} onChange={(event) => updateFilter(index, { operator: event.target.value, value: event.target.value === 'exists' ? '' : filter.value })}>
                      {operatorsForField(filter.field).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                    </select>
                  </Field>
                  {filter.operator !== 'exists' ? (
                    <Field label={`Filter value ${index + 1}`}>
                      <input className="text-input" value={filter.value} onChange={(event) => updateFilter(index, { value: event.target.value })} placeholder={temporalFields.has(filter.field) ? '2026-12-31 (UTC)' : 'open'} />
                    </Field>
                  ) : null}
                  <Button className="button-secondary" type="button" onClick={() => removeFilter(index)}>Remove</Button>
                </div>
              ))}
            </div>

            <Field label="Group by">
              <select className="text-input" value={form.groupBy} disabled={form.aggregationFunction === 'none'} onChange={(event) => setForm({ ...form, groupBy: event.target.value })}>
                <option value="">No grouping</option>
                {fieldsForSource(form.sourceType).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Aggregation">
              <select className="text-input" value={form.aggregationFunction} onChange={(event) => setAggregationFunction(event.target.value)}>
                {aggregationOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            {aggregationFields.length > 0 ? (
              <Field label="Aggregation field">
                <select className="text-input" value={form.aggregationField} onChange={(event) => setForm({ ...form, aggregationField: event.target.value })}>
                  {aggregationFields.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
                </select>
              </Field>
            ) : null}
            <label className="field-hint">
              <input type="checkbox" checked={form.isActive} onChange={(event) => setForm({ ...form, isActive: event.target.checked })} /> Active report definition
            </label>
            <div>
              <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving...' : editingId ? 'Save report definition' : 'Create report definition'}</Button>
              {editingId ? <Button className="button-secondary" type="button" onClick={resetForm}>Cancel</Button> : null}
            </div>
          </form>
        </Card>
      ) : null}
    </section>
  )
}

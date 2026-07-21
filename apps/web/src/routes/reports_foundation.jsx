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
import {
  aggregationFieldOptions,
  aggregationOptionsForVisualization,
  defaultColumns,
  emptyFilter,
  emptyForm,
  fieldsForSource,
  formWithVisualization,
  formFromDefinition,
  isExecutableReportDefinition,
  operatorsForField,
  payloadFromForm,
  reportSummary,
  sourceLabel,
  sourceOptions,
  temporalFields,
  visualizationLabel,
  visualizationOptions
} from './report_definition_model'

export function ReportsFoundationRoute() {
  const { session, canWrite: canManage } = useAuth()
  const canExport = ['owner', 'admin'].includes(session?.membership?.role)
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
    setForm((current) => formWithVisualization({
      ...current, sourceType, columns: defaultColumns(sourceType), filters: [], groupBy: '', aggregationFunction: 'count', aggregationField: ''
    }, current.visualizationType))
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
    : definitions.filter(isExecutableReportDefinition)

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
              <p>Build, run, and export bounded saved table or grouped bar reports for {session?.organization?.name || 'your team'}. Other charts, dashboards, sharing, and scheduling remain hidden.</p>
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
                  <p className="field-hint">Create a table or grouped bar report with bounded, tenant-safe filters and pagination.</p>
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
                  <span className="chip">{isExecutableReportDefinition(definition) ? `Executable ${definition.visualizationType}` : 'Definition only'}</span>
                  {canManage && (import.meta.env.DEV || isExecutableReportDefinition(definition)) ? <Button className="button-secondary" type="button" onClick={() => startEdit(definition)}>Edit</Button> : null}
                </div>
                <CustomReportResults definition={definition} canExport={canExport} />
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
              <p className="field-hint">Tables show selected fields. Grouped bars require one category plus a count, sum, or average and always include an exact accessible data table.</p>
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
              <select className="text-input" value={form.visualizationType} onChange={(event) => setForm(formWithVisualization(form, event.target.value))}>
                {visualizationOptions.map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>

            {form.visualizationType === 'table' ? <div className="card-stack">
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
            </div> : null}

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

            <Field label={form.visualizationType === 'bar' ? 'Category (group by)' : 'Group by'}>
              <select className="text-input" value={form.groupBy} required={form.visualizationType === 'bar'} disabled={form.aggregationFunction === 'none'} onChange={(event) => setForm({ ...form, groupBy: event.target.value })}>
                {form.visualizationType === 'table' ? <option value="">No grouping</option> : null}
                {fieldsForSource(form.sourceType).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Aggregation">
              <select className="text-input" value={form.aggregationFunction} onChange={(event) => setAggregationFunction(event.target.value)}>
                {aggregationOptionsForVisualization(form.visualizationType, form.sourceType).map((option) => <option key={option.value} value={option.value}>{option.label}</option>)}
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

import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getDataQualitySummary } from '../lib/reports'

const recordPaths = { contact: 'contacts', company: 'companies', deal: 'deals', task: 'tasks' }
const thresholdOptions = [14, 30, 60, 90]

function formatTimestamp(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Unknown update time' : `Updated ${date.toLocaleDateString()}`
}

function formatGeneratedAt(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : `Generated ${date.toLocaleString()}`
}

function businessLabel(value) {
  if (value === 'services') return 'service-team'
  if (value === 'construction-services') return 'construction-service'
  if (value === 'product-sales') return 'product-sales'
  return 'general CRM'
}

export function DataQualityPanel() {
  const [summary, setSummary] = useState({ reports: [], staleDays: 30, businessType: 'general' })
  const [staleDays, setStaleDays] = useState(30)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const controller = new AbortController()
    setIsLoading(true)
    getDataQualitySummary({ staleDays, signal: controller.signal })
      .then((result) => {
        setSummary(result)
        setError('')
      })
      .catch((loadError) => {
        if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load data quality reports.')
      })
      .finally(() => {
        if (!controller.signal.aborted) setIsLoading(false)
      })
    return () => controller.abort()
  }, [staleDays])

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h2>Data quality</h2>
            <p>Explainable cleanup queues for active records, including {businessLabel(summary.businessType)} rules.</p>
          </div>
          <Field label="Stale-deal window">
            <select className="text-input" value={staleDays} onChange={(event) => setStaleDays(Number(event.target.value))}>
              {thresholdOptions.map((days) => <option key={days} value={days}>{days} days</option>)}
            </select>
          </Field>
        </div>
        {isLoading ? <p className="field-hint" role="status">Checking CRM data quality...</p> : null}
        {error ? <InlineError message={error} /> : null}
        {!isLoading && !error && formatGeneratedAt(summary.generatedAt) ? <p className="field-hint">{formatGeneratedAt(summary.generatedAt)}</p> : null}
        <div className="record-list" role="list" aria-label="Data quality reports">
          {!isLoading && !error && summary.reports.length === 0 ? <article className="record-row" role="listitem"><p>No data quality reports are available.</p></article> : null}
          {summary.reports.map((report) => (
            <article className={report.count > 0 ? 'record-row record-row-alert' : 'record-row'} key={report.key} role="listitem">
              <div className="card-stack">
                <div>
                  <h3>{report.title} · {report.count}</h3>
                  <p className="field-hint">{report.description}</p>
                </div>
                {report.count === 0 ? <p>No matching issues.</p> : (
                  <ul className="quality-record-list" aria-label={`${report.title} records`}>
                    {report.records.map((record) => (
                      <li key={`${record.entityType}-${record.entityId}`}>
                        <Link to={`/${recordPaths[record.entityType]}/${record.entityId}`}>{record.label}</Link>
                        <span className="field-hint"> — {record.detail} · {formatTimestamp(record.updatedAt)}</span>
                      </li>
                    ))}
                  </ul>
                )}
                {report.count > report.records.length ? <p className="field-hint">Showing the first {report.records.length} of {report.count}; resolve these records, then refresh for the next batch.</p> : null}
              </div>
            </article>
          ))}
        </div>
      </div>
    </Card>
  )
}

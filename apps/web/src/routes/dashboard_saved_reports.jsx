import { useEffect, useRef, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getSharedReportDashboardResults } from '../lib/reports'
import { CustomReportBar } from './custom_report_bar'

function generatedLabel(value) {
  const generated = new Date(value)
  return Number.isNaN(generated.getTime()) ? '' : generated.toLocaleString()
}

export function DashboardSavedReports() {
  const navigate = useNavigate()
  const [execution, setExecution] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const requestRef = useRef(null)
  const requestVersion = useRef(0)

  async function loadDashboardReports() {
    requestRef.current?.abort()
    const controller = new AbortController()
    requestRef.current = controller
    const version = requestVersion.current + 1
    requestVersion.current = version
    setIsLoading(true)
    setError('')
    try {
      const loaded = await getSharedReportDashboardResults({ signal: controller.signal })
      if (controller.signal.aborted || requestVersion.current !== version) return
      setExecution(loaded)
    } catch (loadError) {
      if (!isAbortError(loadError) && requestVersion.current === version) {
        setExecution(null)
        setError(loadError.message || 'Unable to run the shared report dashboard.')
      }
    } finally {
      if (requestRef.current === controller) requestRef.current = null
      if (!controller.signal.aborted && requestVersion.current === version) setIsLoading(false)
    }
  }

  useEffect(() => {
    loadDashboardReports()
    return () => requestRef.current?.abort()
  }, [])

  const generatedAt = execution ? generatedLabel(execution.generatedAt) : ''
  return (
    <section className="dashboard-grid report-dashboard-grid" aria-labelledby="shared-report-dashboard-heading">
      <div className="section-header report-dashboard-heading">
        <div>
          <p className="eyebrow">Shared analytics</p>
          <h2 id="shared-report-dashboard-heading">Report dashboard</h2>
          <p>{generatedAt ? `All charts use one workspace snapshot generated ${generatedAt}.` : 'Published grouped-bar reports appear here in one workspace snapshot.'}</p>
        </div>
        <div className="button-row">
          <Button className="button-secondary" type="button" disabled={isLoading} onClick={loadDashboardReports}>{isLoading ? 'Refreshing reports…' : 'Refresh reports'}</Button>
          <Button className="button-secondary" type="button" onClick={() => navigate('/reports')}>Manage reports</Button>
        </div>
      </div>
      {isLoading && !execution ? <Card className="report-dashboard-widget-full"><p className="field-hint" role="status">Running shared report dashboard…</p></Card> : null}
      {error ? (
        <Card className="report-dashboard-widget-full">
          <div className="card-stack">
            <InlineError message={error} onRetry={loadDashboardReports} retryLabel="Retry shared dashboard" />
            <p className="field-hint">If a saved report was deactivated or changed, remove or replace it from Reports.</p>
          </div>
        </Card>
      ) : null}
      {!isLoading && execution && execution.widgets.length === 0 ? (
        <Card className="report-dashboard-widget-full">
          <div className="card-stack">
            <div>
              <h3>No shared report charts yet</h3>
              <p>Build an active grouped-bar report, then choose it in the shared dashboard configuration on Reports.</p>
            </div>
            <Button className="button-secondary" type="button" onClick={() => navigate('/reports')}>Open Reports</Button>
          </div>
        </Card>
      ) : null}
      {execution?.widgets.map((widget) => (
        <Card className={`report-dashboard-widget report-dashboard-widget-${widget.width}`} key={widget.definition.id}>
          <article className="card-stack" aria-labelledby={`dashboard-report-${widget.definition.id}`}>
            <div>
              <h3 id={`dashboard-report-${widget.definition.id}`}>{widget.definition.name}</h3>
              {widget.definition.description ? <p>{widget.definition.description}</p> : null}
            </div>
            <CustomReportBar definition={widget.definition} result={widget.result} />
            {widget.result.hasMore ? <p className="field-hint" role="status">Showing the first 12 categories. Open the saved report for the complete paged result or CSV export.</p> : null}
            <Button className="button-secondary" type="button" onClick={() => navigate('/reports')}>Open saved report</Button>
          </article>
        </Card>
      ))}
    </section>
  )
}

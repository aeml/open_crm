import { useEffect, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getReportResults, reportExportURL } from '../lib/reports'
import { CustomReportBar } from './custom_report_bar'
import { CustomReportTable } from './custom_report_table'
import { isExecutableReportDefinition } from './report_definition_model'

function generatedLabel(value) {
  if (!value) return ''
  const generated = new Date(value)
  if (Number.isNaN(generated.getTime())) return ''
  return generated.toLocaleString()
}

export function CustomReportResults({ definition, canExport = false }) {
  const [result, setResult] = useState(null)
  const [page, setPage] = useState(1)
  const [isLoading, setIsLoading] = useState(false)
  const [error, setError] = useState('')
  const requestRef = useRef(null)

  useEffect(() => {
    requestRef.current?.abort()
    setResult(null)
    setPage(1)
    setError('')
    setIsLoading(false)
    return () => requestRef.current?.abort()
  }, [definition.id, definition.updatedAt])

  async function runReport(nextPage = page) {
    requestRef.current?.abort()
    const controller = new AbortController()
    requestRef.current = controller
    setIsLoading(true)
    setError('')
    try {
      const nextResult = await getReportResults(definition.id, { page: nextPage, pageSize: 50, signal: controller.signal })
      if (nextResult.definitionId !== definition.id || nextResult.visualizationType !== definition.visualizationType || nextResult.visualizationContract !== (definition.visualizationContract || '') || nextResult.sourceType !== definition.sourceType) {
        throw new Error('The saved report returned results for a different definition.')
      }
      setResult(nextResult)
      setPage(nextResult.page || nextPage)
    } catch (runError) {
      if (!isAbortError(runError)) {
        setError(runError.message || 'Unable to run saved report.')
      }
    } finally {
      if (requestRef.current === controller) {
        requestRef.current = null
        setIsLoading(false)
      }
    }
  }

  if (!isExecutableReportDefinition(definition)) {
    return <p className="field-hint">This saved visualization remains hidden from execution until its chart renderer is complete.</p>
  }
  if (!definition.isActive) {
    return <p className="field-hint">Activate this report before running it.</p>
  }

  return (
    <div className="custom-report-results card-stack">
      <div className="button-row">
        <Button className="button-secondary" type="button" disabled={isLoading} onClick={() => runReport(page)}>
          {isLoading ? 'Running…' : result ? 'Refresh results' : 'Run report'}
        </Button>
        {canExport ? <a className="button button-secondary" href={reportExportURL(definition.id)}>Download CSV</a> : null}
      </div>
      {error ? <InlineError message={error} onRetry={() => runReport(page)} retryLabel="Retry report" /> : null}
      {result ? (
        <>
          {definition.visualizationType === 'bar' ? <CustomReportBar definition={definition} result={result} /> : <CustomReportTable definition={definition} result={result} />}
          <div className="section-header">
            <p className="field-hint" role="status">
              Page {result.page} · {result.rows.length} result{result.rows.length === 1 ? '' : 's'}{generatedLabel(result.generatedAt) ? ` · generated ${generatedLabel(result.generatedAt)}` : ''}
            </p>
            <div className="button-row">
              <Button className="button-secondary" type="button" disabled={isLoading || result.page <= 1} onClick={() => runReport(result.page - 1)}>Previous page</Button>
              <Button className="button-secondary" type="button" disabled={isLoading || !result.hasMore} onClick={() => runReport(result.page + 1)}>Next page</Button>
            </div>
          </div>
        </>
      ) : null}
    </div>
  )
}

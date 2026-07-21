import { useEffect, useRef, useState } from 'react'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { getReportResults } from '../lib/reports'

function displayValue(value) {
  if (value === null || value === undefined || value === '') return '—'
  return String(value)
}

function generatedLabel(value) {
  if (!value) return ''
  const generated = new Date(value)
  if (Number.isNaN(generated.getTime())) return ''
  return generated.toLocaleString()
}

export function CustomReportResults({ definition }) {
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

  if (definition.visualizationType !== 'table') {
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
      </div>
      {error ? <InlineError message={error} onRetry={() => runReport(page)} retryLabel="Retry report" /> : null}
      {result ? (
        <>
          <div className="table-scroll" tabIndex="0" role="region" aria-label={`${definition.name} results`}>
            <table className="data-table">
              <caption>{definition.name} · page {result.page}</caption>
              <thead>
                <tr>{result.columns.map((column) => <th scope="col" key={column.key}>{column.label}</th>)}</tr>
              </thead>
              <tbody>
                {result.rows.length === 0 ? (
                  <tr><td colSpan={Math.max(1, result.columns.length)}>No records match this saved report.</td></tr>
                ) : result.rows.map((row, rowIndex) => (
                  <tr key={`${result.page}-${rowIndex}`}>
                    {result.columns.map((column) => <td key={column.key}>{displayValue(row.values?.[column.key])}</td>)}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
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

import { useEffect, useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listDealPipelines } from '../lib/deals'
import { getPipelineFunnelReport } from '../lib/reports'
import { listOrganizationUsers } from '../lib/users'

function dateInputValue(date) {
  return date.toISOString().slice(0, 10)
}

function defaultDates() {
  const asOf = new Date()
  const from = new Date(asOf)
  from.setUTCDate(from.getUTCDate() - 29)
  return { from: dateInputValue(from), to: dateInputValue(asOf), asOf: dateInputValue(asOf) }
}

function firstEntryStage(pipeline) {
  return pipeline?.stages?.find((stage) => !stage.isClosed) || pipeline?.stages?.[0]
}

function reportMatchesQuery(report, query) {
  return Number(report.pipelineId) === Number(query.pipelineId) && Number(report.entryStageId) === Number(query.entryStageId) && report.fromDate === query.from && report.toDate === query.to && report.asOfDate === query.asOf && Number(report.ownerUserId || 0) === Number(query.ownerUserId || 0)
}

function metric(value) {
  return Number(value || 0).toLocaleString()
}

function percent(value) {
  return value ? `${value}%` : '—'
}

function days(value) {
  return value === '' || value == null ? '—' : `${value} days`
}

export function PipelineFunnelReport() {
  const [pipelines, setPipelines] = useState([])
  const [users, setUsers] = useState([])
  const [optionsError, setOptionsError] = useState('')
  const [optionsRun, setOptionsRun] = useState(0)
  const [draft, setDraft] = useState(() => ({ ...defaultDates(), pipelineId: '', entryStageId: '', ownerUserId: '' }))
  const [query, setQuery] = useState(null)
  const [report, setReport] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    const controller = new AbortController()
    Promise.all([
      listDealPipelines({ signal: controller.signal }),
      listOrganizationUsers({ includeDisabled: true, signal: controller.signal })
    ]).then(([pipelineResult, userResult]) => {
      setPipelines(pipelineResult)
      setUsers(userResult)
      setOptionsError('')
      const pipeline = pipelineResult.find((candidate) => candidate.isDefault) || pipelineResult[0]
      const entryStage = firstEntryStage(pipeline)
      if (!pipeline || !entryStage) {
        setIsLoading(false)
        return
      }
      const initial = { ...defaultDates(), pipelineId: String(pipeline.id), entryStageId: String(entryStage.id), ownerUserId: '' }
      setDraft(initial)
      setQuery({ ...initial, run: 0 })
    }).catch((loadError) => {
      if (!isAbortError(loadError)) {
        setOptionsError(loadError.message || 'Unable to load pipeline report options.')
        setIsLoading(false)
      }
    })
    return () => controller.abort()
  }, [optionsRun])

  useEffect(() => {
    if (!query) return undefined
    const controller = new AbortController()
    setIsLoading(true)
    getPipelineFunnelReport({ ...query, signal: controller.signal })
      .then((result) => {
        if (!reportMatchesQuery(result, query)) throw new Error('The pipeline report returned a different cohort or observation window.')
        setReport(result)
        setError('')
      })
      .catch((loadError) => { if (!isAbortError(loadError)) setError(loadError.message || 'Unable to load pipeline conversion and velocity.') })
      .finally(() => { if (!controller.signal.aborted) setIsLoading(false) })
    return () => controller.abort()
  }, [query])

  function selectPipeline(pipelineId) {
    const pipeline = pipelines.find((candidate) => String(candidate.id) === pipelineId)
    setDraft({ ...draft, pipelineId, entryStageId: String(firstEntryStage(pipeline)?.id || '') })
  }

  function runReport(event) {
    event?.preventDefault()
    setQuery({ ...draft, run: (query?.run || 0) + 1 })
  }

  const selectedPipeline = pipelines.find((pipeline) => String(pipeline.id) === String(draft.pipelineId))
  const totals = report?.totals || {}
  const summary = [
    ['Cohort deals', totals.cohortDeals],
    ['Open as of date', totals.openDeals],
    ['Won as of date', totals.wonDeals],
    ['Lost as of date', totals.lostDeals],
    ['Closed win rate', percent(totals.winRatePercent)],
    ['Median time to win', days(totals.medianDaysToWin)],
    ['Open in another pipeline', totals.movedOutOpenDeals]
  ]

  return (
    <Card className="pipeline-funnel-report-card">
      <div className="card-stack" aria-label="Pipeline conversion and velocity">
        <div>
          <h2>Pipeline conversion and velocity</h2>
          <p>A real deal cohort from one exact entry stage, measured through a fixed as-of date.</p>
        </div>
        <form className="sales-report-filters" onSubmit={runReport}>
          <Field label="Pipeline">
            <select className="text-input" value={draft.pipelineId} onChange={(event) => selectPipeline(event.target.value)} required>
              <option value="">Choose a pipeline</option>
              {pipelines.map((pipeline) => <option key={pipeline.id} value={pipeline.id}>{pipeline.name}</option>)}
            </select>
          </Field>
          <Field label="Entry stage">
            <select className="text-input" value={draft.entryStageId} onChange={(event) => setDraft({ ...draft, entryStageId: event.target.value })} required>
              <option value="">Choose an entry stage</option>
              {(selectedPipeline?.stages || []).map((stage) => <option key={stage.id} value={stage.id}>{stage.name}{stage.isClosed ? ' (closed)' : ''}</option>)}
            </select>
          </Field>
          <Field label="Cohort from (UTC)">
            <input className="text-input" type="date" value={draft.from} max={draft.to} onChange={(event) => setDraft({ ...draft, from: event.target.value })} required />
          </Field>
          <Field label="Cohort to (UTC)">
            <input className="text-input" type="date" value={draft.to} min={draft.from} max={draft.asOf} onChange={(event) => setDraft({ ...draft, to: event.target.value })} required />
          </Field>
          <Field label="Observe through (UTC)">
            <input className="text-input" type="date" value={draft.asOf} min={draft.to} max={dateInputValue(new Date())} onChange={(event) => setDraft({ ...draft, asOf: event.target.value })} required />
          </Field>
          <Field label="Owner at creation">
            <select className="text-input" value={draft.ownerUserId} onChange={(event) => setDraft({ ...draft, ownerUserId: event.target.value })}>
              <option value="">Everyone</option>
              {users.map((user) => <option key={user.id} value={user.id}>{user.firstName} {user.lastName}{user.status === 'disabled' ? ' (disabled)' : ''}</option>)}
            </select>
          </Field>
          <Button type="submit" disabled={isLoading || !draft.pipelineId || !draft.entryStageId}>{isLoading ? 'Running...' : 'Run pipeline report'}</Button>
        </form>
        {optionsError ? <InlineError message={optionsError} onRetry={() => setOptionsRun((current) => current + 1)} retryLabel="Retry report options" /> : null}
        {error ? <InlineError message={error} onRetry={runReport} retryLabel="Retry pipeline report" /> : null}
        {isLoading ? <p className="field-hint" role="status">Running pipeline cohort report...</p> : null}
        {!isLoading && !optionsError && pipelines.length === 0 ? <p className="inline-note">Configure a deal pipeline and entry stage before running this report.</p> : null}
        {!isLoading && !error && report ? (
          <>
            <p className={report.historyComplete ? 'inline-note' : 'inline-note sales-report-coverage-warning'} role="status">
              {report.historyComplete ? `Complete cohort event coverage from ${report.fromDate} through ${report.asOfDate} UTC.` : `Partial cohort history: tracking began ${new Date(report.coverageStartedAt).toLocaleString()}. Earlier creation or stage events are not inferred.`}
            </p>
            <p className="inline-note">{report.pipelineName} / {report.entryStageName}: {metric(totals.cohortDeals)} deal{totals.cohortDeals === 1 ? '' : 's'} created from {report.fromDate} through {report.toDate}, observed through {report.asOfDate} UTC.</p>
            <div className="sales-report-metrics" role="list" aria-label="Pipeline cohort totals">
              {summary.map(([label, value]) => <div className="sales-report-metric" role="listitem" key={label}><p className="metric-label">{label}</p><p className="metric-value">{typeof value === 'number' || value == null ? metric(value) : value}</p></div>)}
            </div>
            <div className="table-scroll" tabIndex="0" role="region" aria-label="Pipeline cohort stage metrics">
              <table className="data-table">
                <caption>Exact stage reach and elapsed-time metrics for the selected deal cohort</caption>
                <thead><tr><th scope="col">Current stage definition</th><th scope="col">Reached</th><th scope="col">Current</th><th scope="col">Exited</th><th scope="col">Forward or won</th><th scope="col">Lost exits</th><th scope="col">Median to reach</th><th scope="col">Median completed visit</th></tr></thead>
                <tbody>
                  {report.stages.map((stage) => (
                    <tr key={stage.stageId}>
                      <th scope="row">{stage.stageName} <span className="field-hint">({stage.stageOutcome})</span></th>
                      <td>{metric(stage.reachedDeals)} · {percent(stage.reachRatePercent)}</td>
                      <td>{metric(stage.currentlyInStageDeals)}</td>
                      <td>{metric(stage.exitedDeals)}</td>
                      <td>{metric(stage.forwardOrWonDeals)} · {percent(stage.forwardExitRatePercent)}</td>
                      <td>{metric(stage.lostExitDeals)}</td>
                      <td>{days(stage.medianDaysToReach)}</td>
                      <td>{days(stage.medianDaysInCompletedVisit)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <details><summary>How this cohort and velocity report is calculated</summary><ul>{report.semantics.map((rule) => <li key={rule}>{rule}</li>)}</ul></details>
          </>
        ) : null}
      </div>
    </Card>
  )
}

import { useEffect, useMemo, useRef, useState } from 'react'
import { useAuth } from '../app/providers'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { InlineError } from '../components/ui/inline_error'
import { isAbortError } from '../lib/api'
import { listBackgroundJobs, replayBackgroundJob, resolveSequenceDelivery } from '../lib/background_jobs'
import { crmExportDownloadURL, initialCRMExportRequest, listCRMExports, requestCRMExport } from '../lib/crm_exports'
import { createIdempotencyKey } from '../lib/idempotency'
import { usePageTitle } from '../lib/use_page_title'

const statusOptions = [
  { value: '', label: 'All statuses' },
  { value: 'dead', label: 'Needs attention' },
  { value: 'retryable', label: 'Waiting to retry' },
  { value: 'running', label: 'Running' },
  { value: 'pending', label: 'Pending' },
  { value: 'succeeded', label: 'Succeeded' }
]

function formatTimestamp(value) {
  if (!value) {
    return 'Not recorded'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not recorded' : date.toLocaleString()
}

function jobLabel(type) {
  if (type === 'calendar.reminder') return 'Calendar reminder'
  if (type === 'mailbox.sync') return 'Mailbox sync'
  if (type === 'email_sequence.send') return 'Email sequence send'
  if (type === 'billing.reconcile') return 'Billing reconciliation'
  if (type === 'billing.usage.snapshot') return 'Billing usage snapshot'
  if (type === 'workspace.export.generate') return 'Workspace export'
  if (type === 'crm.export.generate') return 'Filtered CRM export'
  if (type === 'workflow.lead_follow_up') return 'Lead follow-up automation'
  return type || 'Background job'
}

function requiresSequenceReview(job) {
  return job?.type === 'email_sequence.send' && /uncertain|smtp.*state finalization/i.test(job?.lastError || '')
}

export function SettingsOperationsRoute() {
  const { session, workspaceWritable } = useAuth()
  usePageTitle('Operations')
  const role = session?.membership?.role || ''
  const canOperate = useMemo(() => ['owner', 'admin'].includes(role), [role])
  const [jobs, setJobs] = useState([])
  const [stats, setStats] = useState({})
  const [status, setStatus] = useState('dead')
  const [type, setType] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [isLoading, setIsLoading] = useState(true)
  const [replayingId, setReplayingId] = useState(0)
  const [resolvingId, setResolvingId] = useState(0)
  const [exports, setExports] = useState([])
  const [exportRequest] = useState(initialCRMExportRequest)
  const exportRequestKey = useRef(createIdempotencyKey('crm-export'))
  const [isRequestingExport, setIsRequestingExport] = useState(false)

  async function load({ signal } = {}) {
    if (!canOperate) {
      setError('Admin access required')
      setJobs([])
      setIsLoading(false)
      return
    }
    setIsLoading(true)
    try {
      const [jobsResult, exportsResult] = await Promise.allSettled([listBackgroundJobs({ status, type, signal }), listCRMExports({ signal })])
      if (signal?.aborted) return
      const failures = []
      if (jobsResult.status === 'fulfilled') {
        setJobs(jobsResult.value.jobs)
        setStats(jobsResult.value.stats)
      } else if (!isAbortError(jobsResult.reason)) {
        setJobs([])
        setStats({})
        failures.push(jobsResult.reason?.message || 'Unable to load background jobs.')
      }
      if (exportsResult.status === 'fulfilled') {
        setExports(exportsResult.value)
      } else if (!isAbortError(exportsResult.reason)) {
        failures.push(exportsResult.reason?.message || 'Unable to load CRM exports.')
      }
      setError(failures.join(' '))
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    load({ signal: controller.signal })
    return () => controller.abort()
  }, [canOperate, status, type])

  const exportGenerating = exports.some((item) => item.status === 'pending' || item.status === 'processing')
  useEffect(() => {
    if (!exportGenerating) return undefined
    const controller = new AbortController()
    const timer = window.setTimeout(() => load({ signal: controller.signal }), 2000)
    return () => {
      window.clearTimeout(timer)
      controller.abort()
    }
  }, [exportGenerating, exports])

  async function handleReplay(job) {
    setReplayingId(job.id)
    setNotice('')
    setError('')
    try {
      await replayBackgroundJob(job.id)
      setNotice(`${jobLabel(job.type)} queued for replay.`)
      await load()
    } catch (replayError) {
      setError(replayError.message || 'Unable to replay background job.')
    } finally {
      setReplayingId(0)
    }
  }

  async function handleSequenceResolution(job, resolution) {
    const retrying = resolution === 'retry'
    const prompt = retrying
      ? 'Retrying may send a duplicate if SMTP accepted the earlier attempt. Retry only after checking the sender mailbox. Continue?'
      : 'Confirm that the message is visible in the sender mailbox. This advances the sequence without sending again. Continue?'
    if (!window.confirm(prompt)) {
      return
    }
    setResolvingId(job.id)
    setNotice('')
    setError('')
    try {
      await resolveSequenceDelivery(job.id, resolution)
      setNotice(retrying ? 'Sequence email queued for one operator-approved retry.' : 'Sequence email confirmed sent without another SMTP attempt.')
      await load()
    } catch (resolutionError) {
      setError(resolutionError.message || 'Unable to resolve sequence delivery.')
    } finally {
      setResolvingId(0)
    }
  }

  async function handleExportRequest() {
    setIsRequestingExport(true)
    setNotice('')
    setError('')
    try {
      await requestCRMExport(exportRequest, exportRequestKey.current)
      exportRequestKey.current = createIdempotencyKey('crm-export')
      setNotice('Filtered CRM export queued. Follow it below or recover it in the job ledger.')
      await load()
    } catch (requestError) {
      setError(requestError.message || 'Unable to request the CRM export.')
    } finally {
      setIsRequestingExport(false)
    }
  }

  return (
    <section className="dashboard-grid settings-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Background operations</h2>
              <p>Inspect durable reminders, mailbox sync, billing, imports, exports, automation, and sequence delivery work for {session?.organization?.name || 'your workspace'}.</p>
            </div>
            <Button className="button-secondary" type="button" onClick={() => load()} disabled={!canOperate || isLoading}>Refresh</Button>
          </div>

          <div className="record-list" role="list" aria-label="Background job health">
            <article className={stats.dead > 0 ? 'record-row record-row-alert' : 'record-row'} role="listitem">
              <div><h3>{stats.dead || 0} need attention</h3><p className="field-hint">Dead jobs stay visible until an admin reviews and replays them.</p></div>
            </article>
            <article className="record-row" role="listitem">
              <div><h3>{stats.running || 0} running · {stats.retryable || 0} retrying · {stats.pending || 0} pending</h3><p className="field-hint">Oldest ready job: {formatTimestamp(stats.oldestReadyAt)}</p></div>
            </article>
          </div>

          <div className="form-grid form-grid-two">
            <Field label="Job status">
              <select className="text-input" value={status} onChange={(event) => setStatus(event.target.value)} disabled={!canOperate}>
                {statusOptions.map((option) => <option key={option.value || 'all'} value={option.value}>{option.label}</option>)}
              </select>
            </Field>
            <Field label="Job type">
              <select className="text-input" value={type} onChange={(event) => setType(event.target.value)} disabled={!canOperate}>
                <option value="">All job types</option>
                <option value="calendar.reminder">Calendar reminders</option>
                <option value="mailbox.sync">Mailbox sync</option>
                <option value="billing.reconcile">Billing reconciliation</option>
                <option value="billing.usage.snapshot">Billing usage snapshots</option>
                <option value="workspace.export.generate">Workspace exports</option>
                <option value="crm.export.generate">Filtered CRM exports</option>
                <option value="import.execute">CRM imports</option>
                <option value="email_sequence.send">Email sequence sends</option>
                <option value="workflow.lead_follow_up">Lead follow-up automations</option>
              </select>
            </Field>
          </div>

          {isLoading ? <p className="field-hint" role="status">Loading background jobs...</p> : null}
          {error ? <InlineError message={error} /> : null}
          {notice ? <div className="inline-note" role="status">{notice}</div> : null}
          <div className="record-list" role="list" aria-label="Background jobs">
            {!isLoading && jobs.length === 0 ? (
              <article className="record-row" role="listitem">
                <div><p>No jobs match these filters.</p><p className="field-hint">A clear needs-attention view means no failed operation currently requires admin action.</p></div>
              </article>
            ) : jobs.map((job) => {
              const reviewRequired = requiresSequenceReview(job)
              return (
                <article className={job.status === 'dead' ? 'record-row record-row-alert' : 'record-row'} key={job.id} role="listitem">
                  <div>
                    <h3>{jobLabel(job.type)} · {job.status}</h3>
                    <p className="field-hint">Attempt {job.attempts} of {job.maxAttempts} · queued {formatTimestamp(job.createdAt)}</p>
                    {job.lastError ? <p>{job.lastError}</p> : null}
                    {reviewRequired ? <p className="field-hint">Delivery may already have reached SMTP. Check the sender mailbox, then record whether it arrived or explicitly approve one retry.</p> : null}
                  </div>
                  {job.status === 'dead' && reviewRequired ? (
                    <div className="button-row">
                      <Button className="button-secondary" type="button" onClick={() => handleSequenceResolution(job, 'confirmed_sent')} disabled={!workspaceWritable || resolvingId === job.id}>Confirm already sent</Button>
                      <Button className="button-secondary" type="button" onClick={() => handleSequenceResolution(job, 'retry')} disabled={!workspaceWritable || resolvingId === job.id}>Retry email</Button>
                    </div>
                  ) : job.status === 'dead' ? (
                    <Button className="button-secondary" type="button" onClick={() => handleReplay(job)} disabled={replayingId === job.id || reviewRequired}>
                      {replayingId === job.id ? 'Replaying...' : 'Replay job'}
                    </Button>
                  ) : null}
                </article>
              )
            })}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack crm-export-card">
          <div className="section-header">
            <div><h2>Filtered CRM exports</h2><p>Queue {exportRequest.resource} with its current list filters. Up to 50,000 rows; files expire after seven days.</p></div>
            <Button type="button" onClick={handleExportRequest} disabled={!canOperate || !workspaceWritable || isRequestingExport || exportGenerating}>{isRequestingExport ? 'Queueing…' : exportGenerating ? 'Generating…' : 'Queue CSV'}</Button>
          </div>
          {exportRequest.search ? <p className="field-hint">Search: {exportRequest.search}. Other list filters remain attached.</p> : null}
          {!workspaceWritable ? <p className="field-hint">New exports are unavailable while this hosted workspace is read-only. Existing ready files remain downloadable.</p> : null}
          {exports.length === 0 && !isLoading ? <p className="field-hint">No filtered CRM exports have been requested.</p> : null}
          <div className="record-list" role="list" aria-label="Filtered CRM export history">
            {exports.map((item) => (
              <article className={item.status === 'failed' ? 'record-row record-row-alert' : 'record-row'} role="listitem" key={item.id}>
                <div>
                  <h3>{item.resource} export #{item.id} · {item.status}</h3>
                  {item.status === 'processing' ? <p className="field-hint" role="status">{Number(item.progressRows).toLocaleString()} rows processed</p> : null}
                  {item.status === 'ready' ? <p className="field-hint">{Number(item.rowCount).toLocaleString()} rows · expires {formatTimestamp(item.expiresAt)}</p> : null}
                  {item.lastError ? <p>{item.lastError}</p> : null}
                </div>
                {item.status === 'ready' ? <a className="button button-secondary" href={crmExportDownloadURL(item.id)}>Download CSV</a> : null}
              </article>
            ))}
          </div>
        </div>
      </Card>

      <Card>
        <div className="card-stack">
          <div><h2>Safe recovery</h2><p>Fix configuration or provider failures before replay. Billing reconciliation re-reads ordered, tenant-matched Stripe state.</p></div>
          <div className="inline-note"><strong>Email safety:</strong> an uncertain sequence send is never retried automatically because SMTP may have accepted it before the connection failed.</div>
        </div>
      </Card>
    </section>
  )
}

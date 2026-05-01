import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { getDashboardSummary } from '../lib/dashboard'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'

function formatMoney(value) {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' }).format(amount)
}

function formatRelativeTimestamp(value) {
  if (!value) {
    return 'Just now'
  }

  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return 'Just now'
  }

  return date.toLocaleString()
}

function parseTimestamp(value) {
  if (!value) {
    return null
  }

  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? null : parsed
}

function daysSince(value) {
  const parsed = parseTimestamp(value)
  if (!parsed) {
    return null
  }

  const elapsed = Date.now() - parsed.getTime()
  return Math.max(0, Math.floor(elapsed / (24 * 60 * 60 * 1000)))
}

function latestActivityFor(activities, entityType) {
  return (activities || []).find((activity) => activity.entityType === entityType) || null
}

function recentlyTouchedRecords(activities) {
  const seen = new Set()
  return (activities || []).filter((activity) => {
    if (activity.entityType !== 'contact' && activity.entityType !== 'company') {
      return false
    }

    const key = `${activity.entityType}:${activity.entityId}`
    if (seen.has(key)) {
      return false
    }
    seen.add(key)
    return true
  }).slice(0, 3)
}

function dashboardLabels(businessType) {
  if (businessType === 'services' || businessType === 'construction-services') {
    return {
      pipelineLabel: 'Open jobs value',
      dueTodayLabel: 'Due today',
      contactsLabel: 'New contacts',
      openRecordsLabel: 'Open jobs',
      wonRecordsLabel: 'Won jobs',
      recordsLower: 'jobs',
      activityDescription: 'The last real changes across jobs, contacts, clients, and service tasks.'
    }
  }

  return {
    pipelineLabel: 'Open pipeline',
    dueTodayLabel: 'Due today',
    contactsLabel: 'New contacts',
    openRecordsLabel: 'Open deals',
    wonRecordsLabel: 'Won deals',
    recordsLower: 'deals',
    activityDescription: 'The last real changes across deals, contacts, companies, and tasks.'
  }
}

export function DashboardRoute() {
  const navigate = useNavigate()
  const { session, businessProfile } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const labels = dashboardLabels(businessType)
  const [summary, setSummary] = useState({
    pipelineValue: '0',
    openDealsCount: 0,
    wonDealsCount: 0,
    openTasksCount: 0,
    dueTodayCount: 0,
    newContactsCount: 0,
    recentActivities: []
  })
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  async function loadSummary({ signal } = {}) {
    setIsLoading(true)
    try {
      const nextSummary = await getDashboardSummary({ signal })
      setSummary(nextSummary)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load dashboard summary.')
      }
    } finally {
      if (!signal?.aborted) {
        setIsLoading(false)
      }
    }
  }

  useEffect(() => {
    const controller = new AbortController()

    loadSummary({ signal: controller.signal })
    return () => {
      controller.abort()
    }
  }, [])

  const heroMetrics = useMemo(
    () => [
      { label: labels.pipelineLabel, value: formatMoney(summary.pipelineValue) },
      { label: labels.dueTodayLabel, value: `${summary.dueTodayCount} tasks` },
      { label: labels.contactsLabel, value: `${summary.newContactsCount} this week` }
    ],
    [labels.contactsLabel, labels.dueTodayLabel, labels.pipelineLabel, summary.dueTodayCount, summary.newContactsCount, summary.pipelineValue]
  )
  const upcomingTasksCount = Math.max(0, Number(summary.openTasksCount || 0) - Number(summary.dueTodayCount || 0))
  const latestPipelineActivity = latestActivityFor(summary.recentActivities, 'deal')
  const latestPipelineDays = daysSince(latestPipelineActivity?.createdAt)
  const touchedRecords = useMemo(() => recentlyTouchedRecords(summary.recentActivities), [summary.recentActivities])
  const pipelineAttention = useMemo(() => {
    if (summary.openDealsCount <= 0) {
      return {
        title: `No open ${labels.recordsLower}`,
        description: 'The active pipeline is clear. Review recent activity or create the next opportunity when work appears.',
        action: 'Review activity',
        path: '/deals'
      }
    }

    if (!latestPipelineActivity) {
      return {
        title: `${summary.openDealsCount} open ${labels.recordsLower} need a touch`,
        description: `No recent ${labels.recordsLower} activity appears in the dashboard feed. Check stage age and next steps before work stalls.`,
        action: `Review ${labels.recordsLower}`,
        path: '/deals'
      }
    }

    if (latestPipelineDays !== null && latestPipelineDays >= 7) {
      return {
        title: `Pipeline has been quiet for ${latestPipelineDays} days`,
        description: `The latest ${labels.recordsLower.slice(0, -1)} activity was ${latestPipelineActivity.summary.toLowerCase()}. Confirm the next move.`,
        action: `Review ${labels.recordsLower}`,
        path: '/deals'
      }
    }

    return {
      title: 'Pipeline touched recently',
      description: `${latestPipelineActivity.summary} is the latest pipeline signal. Keep the next task tied to the record.`,
      action: `Open ${labels.recordsLower}`,
      path: '/deals'
    }
  }, [labels.recordsLower, latestPipelineActivity, latestPipelineDays, summary.openDealsCount])
  const hasWorkspaceData = Number.parseFloat(summary.pipelineValue || '0') > 0 || summary.openDealsCount > 0 || summary.wonDealsCount > 0 || summary.openTasksCount > 0 || summary.dueTodayCount > 0 || summary.newContactsCount > 0 || summary.recentActivities.length > 0
  const setupSteps = useMemo(() => {
    const pipelineLabel = businessType === 'services' || businessType === 'construction-services' ? 'Create your first job' : 'Create your first deal'
    return [
      { label: 'Add a contact', description: 'Start with the person you need to follow up with.', action: 'Add contact', path: '/contacts' },
      { label: 'Add a client', description: 'Create the company or individual client behind the work.', action: 'Add client', path: '/companies' },
      { label: pipelineLabel, description: 'Track the opportunity, job, or active revenue conversation.', action: pipelineLabel, path: '/deals' }
    ]
  }, [businessType])

  return (
    <>
      <section className="card hero-card">
        <div className="hero-grid">
          {heroMetrics.map((metric) => (
            <div key={metric.label}>
              <p className="metric-label">{metric.label}</p>
              <p className="metric-value">{metric.value}</p>
            </div>
          ))}
        </div>
      </section>

      {!isLoading && !error && !hasWorkspaceData ? (
        <Card>
          <div className="card-stack">
            <div>
              <p className="eyebrow">Start your workspace</p>
              <h2>Build the first useful CRM loop.</h2>
              <p>Add one person, connect the client, then track the deal or job. Once those records exist, dashboard activity and follow-up work will appear here automatically.</p>
            </div>
            <div className="step-list" aria-label="First setup steps">
              {setupSteps.map((step, index) => (
                <article className="step-row" key={step.label}>
                  <span className="step-number">{index + 1}</span>
                  <div>
                    <p>{step.label}</p>
                    <p className="field-hint">{step.description}</p>
                  </div>
                  <div>
                    <Button className="button-secondary" type="button" onClick={() => navigate(step.path)}>{step.action}</Button>
                  </div>
                </article>
              ))}
            </div>
          </div>
        </Card>
      ) : null}

      <section className="dashboard-grid">
        <Card>
          <div className="card-stack">
            <div>
              <p className="eyebrow">Needs attention</p>
              <h2>Task focus</h2>
              <p>Start with time-sensitive work, then use the upcoming queue to keep follow-ups from drifting.</p>
            </div>
            <div className="decision-list" role="list" aria-label="Task decision list">
              <article className="decision-row" role="listitem">
                <div>
                  <p>Overdue check</p>
                  <p className="field-hint">Review anything that slipped before scheduling new work.</p>
                </div>
                <Button className="button-secondary" type="button" onClick={() => navigate('/tasks?due=overdue')}>Review overdue</Button>
              </article>
              <article className="decision-row" role="listitem">
                <div>
                  <p>{summary.dueTodayCount === 1 ? '1 task due today' : `${summary.dueTodayCount} tasks due today`}</p>
                  <p className="field-hint">Close or reschedule work that needs attention today.</p>
                </div>
                <Button type="button" onClick={() => navigate('/tasks?due=dueToday')}>Review due today</Button>
              </article>
              <article className="decision-row" role="listitem">
                <div>
                  <p>{upcomingTasksCount === 1 ? '1 upcoming task' : `${upcomingTasksCount} upcoming tasks`}</p>
                  <p className="field-hint">Protect the next few follow-ups before they become urgent.</p>
                </div>
                <Button className="button-secondary" type="button" onClick={() => navigate('/tasks?due=upcoming')}>Review upcoming</Button>
              </article>
            </div>
          </div>
        </Card>

        <Card>
          <div className="card-stack">
            <div>
              <p className="eyebrow">Pipeline signal</p>
              <h2>{pipelineAttention.title}</h2>
              <p>{pipelineAttention.description}</p>
            </div>
            <div>
              <Button className="button-secondary" type="button" onClick={() => navigate(pipelineAttention.path)}>{pipelineAttention.action}</Button>
            </div>
          </div>
        </Card>

        <Card>
          <div className="card-stack">
            <div>
              <h2>Today</h2>
              <p>See what is live in the pipeline instead of guessing from stale numbers.</p>
            </div>
            {isLoading ? <p className="field-hint">Loading dashboard summary...</p> : null}
            {error ? (
              <div className="card-stack">
                <p className="form-error">{error}</p>
                <div>
                  <Button className="button-secondary" type="button" onClick={() => loadSummary()}>
                    Retry summary
                  </Button>
                </div>
              </div>
            ) : null}
            <div className="record-list" role="list" aria-label="Dashboard summary metrics">
              <article className="record-row" role="listitem">
                <div>
                  <p>{labels.openRecordsLabel}</p>
                </div>
                <div>
                  <p>{summary.openDealsCount}</p>
                </div>
              </article>
              <article className="record-row" role="listitem">
                <div>
                  <p>{labels.wonRecordsLabel}</p>
                </div>
                <div>
                  <p>{summary.wonDealsCount}</p>
                </div>
              </article>
              <article className="record-row" role="listitem">
                <div>
                  <p>Open tasks</p>
                </div>
                <div>
                  <p>{summary.openTasksCount}</p>
                </div>
              </article>
            </div>
            <div className="button-row">
              <Button className="button-secondary" type="button" onClick={() => navigate('/tasks')}>
                Open tasks
              </Button>
            </div>
          </div>
        </Card>

        <Card>
          <div className="card-stack">
            <div>
              <h2>Recent activity</h2>
              <p>{labels.activityDescription}</p>
            </div>
            {summary.recentActivities.length === 0 ? (
              <p className="field-hint">No activity yet. Start creating records and it will show up here.</p>
            ) : (
              <div className="record-list" role="list" aria-label="Recent activity feed">
                {summary.recentActivities.map((activity) => (
                  <article className="record-row" key={activity.id} role="listitem">
                    <div>
                      <p>{activity.summary}</p>
                      <p className="field-hint">{activity.actorName || 'System'} • {activity.entityType} #{activity.entityId}</p>
                    </div>
                    <div>
                      <p>{formatRelativeTimestamp(activity.createdAt)}</p>
                    </div>
                  </article>
                ))}
              </div>
            )}
          </div>
        </Card>

        <Card>
          <div className="card-stack">
            <div>
              <h2>Recently touched contacts and clients</h2>
              <p>Use recent people and account activity to decide who needs the next follow-up.</p>
            </div>
            {touchedRecords.length === 0 ? (
              <p className="field-hint">No recent contact or client touches yet.</p>
            ) : (
              <div className="record-list" role="list" aria-label="Recently touched records">
                {touchedRecords.map((activity) => (
                  <article className="record-row" key={`${activity.entityType}-${activity.entityId}`} role="listitem">
                    <div>
                      <p>{activity.entityType === 'company' ? 'Client' : 'Contact'} #{activity.entityId}</p>
                      <p className="field-hint">{activity.summary}</p>
                    </div>
                    <Button className="button-secondary" type="button" onClick={() => navigate(`/${activity.entityType === 'company' ? 'companies' : 'contacts'}/${activity.entityId}`)}>Open</Button>
                  </article>
                ))}
              </div>
            )}
          </div>
        </Card>
      </section>
    </>
  )
}

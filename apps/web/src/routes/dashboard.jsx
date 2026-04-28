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

function dashboardLabels(businessType) {
  if (businessType === 'services' || businessType === 'construction-services') {
    return {
      pipelineLabel: 'Open jobs value',
      dueTodayLabel: 'Due today',
      contactsLabel: 'New contacts',
      openRecordsLabel: 'Open jobs',
      wonRecordsLabel: 'Won jobs',
      activityDescription: 'The last real changes across jobs, contacts, clients, and service tasks.'
    }
  }

  return {
    pipelineLabel: 'Open pipeline',
    dueTodayLabel: 'Due today',
    contactsLabel: 'New contacts',
    openRecordsLabel: 'Open deals',
    wonRecordsLabel: 'Won deals',
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
              <Button type="button" onClick={() => navigate('/tasks?due=dueToday')}>
                Review due today
              </Button>
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
      </section>
    </>
  )
}

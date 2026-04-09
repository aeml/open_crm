import { useEffect, useMemo, useState } from 'react'
import { Card } from '../components/ui/card'
import { getDashboardSummary } from '../lib/dashboard'

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

export function DashboardRoute() {
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

  useEffect(() => {
    let cancelled = false

    async function loadSummary() {
      try {
        const nextSummary = await getDashboardSummary()
        if (!cancelled) {
          setSummary(nextSummary)
          setError('')
        }
      } catch (loadError) {
        if (!cancelled) {
          setError(loadError.message || 'Unable to load dashboard summary.')
        }
      }
    }

    loadSummary()
    return () => {
      cancelled = true
    }
  }, [])

  const heroMetrics = useMemo(
    () => [
      { label: 'Open pipeline', value: formatMoney(summary.pipelineValue) },
      { label: 'Due today', value: `${summary.dueTodayCount} tasks` },
      { label: 'New contacts', value: `${summary.newContactsCount} this week` }
    ],
    [summary.dueTodayCount, summary.newContactsCount, summary.pipelineValue]
  )

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

      <section className="dashboard-grid">
        <Card>
          <div className="card-stack">
            <div>
              <h2>Today</h2>
              <p>See what is live in the pipeline instead of guessing from stale numbers.</p>
            </div>
            {error ? <p className="form-error">{error}</p> : null}
            <div className="record-list" role="list" aria-label="Dashboard summary metrics">
              <article className="record-row" role="listitem">
                <div>
                  <p>Open deals</p>
                </div>
                <div>
                  <p>{summary.openDealsCount}</p>
                </div>
              </article>
              <article className="record-row" role="listitem">
                <div>
                  <p>Won deals</p>
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
          </div>
        </Card>

        <Card>
          <div className="card-stack">
            <div>
              <h2>Recent activity</h2>
              <p>The last real changes across deals, contacts, companies, and tasks.</p>
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

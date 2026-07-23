import { useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'
import { getDashboardSummary, upsertSalesQuota } from '../lib/dashboard'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import { DashboardForecast } from './dashboard_forecast'
import { DashboardClientReviews } from './dashboard_client_reviews'
import { DashboardSavedReports } from './dashboard_saved_reports'
import {
  dashboardHasWorkspaceData,
  dashboardHeroMetrics,
  dashboardLabels,
  dashboardPipelineAttention,
  dashboardSetupSteps,
  emptyForecast,
  emptySummary,
  formatDashboardTimestamp,
  normalizeDashboardSummary,
  quotaDraftsFromForecast,
  recentlyTouchedRecords
} from './dashboard_model'

export function DashboardRoute() {
  const navigate = useNavigate()
  const { session, businessProfile, canAdminister: canManageQuotas } = useAuth()
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const labels = dashboardLabels(businessType)
  usePageTitle('Dashboard')
  const [summary, setSummary] = useState(emptySummary)
  const [quotaDrafts, setQuotaDrafts] = useState({})
  const [savingQuotaUserId, setSavingQuotaUserId] = useState(null)
  const [forecastPeriod, setForecastPeriod] = useState({ start: '', end: '' })
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  async function loadSummary({ signal, forecastStart = '', forecastEnd = '' } = {}) {
    setIsLoading(true)
    try {
      const nextSummary = await getDashboardSummary({ forecastStart, forecastEnd, signal })
      const normalized = normalizeDashboardSummary(nextSummary)
      setSummary(normalized)
      setQuotaDrafts(quotaDraftsFromForecast(normalized.forecast))
      setForecastPeriod({ start: normalized.forecast.periodStart, end: normalized.forecast.periodEnd })
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

  async function handleSaveQuota(member) {
    const quotaAmount = quotaDrafts[member.userId] ?? member.quotaAmount ?? '0'
    setSavingQuotaUserId(member.userId)
    try {
      const nextSummary = await upsertSalesQuota(member.userId, {
        periodStart: summary.forecast.periodStart,
        periodEnd: summary.forecast.periodEnd,
        quotaAmount,
        currency: summary.forecast.currency || 'USD'
      })
      const normalized = normalizeDashboardSummary(nextSummary)
      setSummary(normalized)
      setQuotaDrafts(quotaDraftsFromForecast(normalized.forecast))
      setError('')
    } catch (quotaError) {
      setError(quotaError.message || 'Unable to save sales quota.')
    } finally {
      setSavingQuotaUserId(null)
    }
  }

  async function handleForecastPeriod(event) {
    event.preventDefault()
    await loadSummary({ forecastStart: forecastPeriod.start, forecastEnd: forecastPeriod.end })
  }

  const heroMetrics = useMemo(
    () => dashboardHeroMetrics(summary, labels),
    [labels.contactsLabel, labels.pipelineLabel, summary.baseCurrency, summary.dueSoonTasksCount, summary.newContactsCount, summary.pipelineValue]
  )
  const touchedRecords = useMemo(() => recentlyTouchedRecords(summary.recentActivities), [summary.recentActivities])
  const forecast = summary.forecast || emptyForecast
  const pipelineAttention = dashboardPipelineAttention(summary, labels)
  const hasWorkspaceData = dashboardHasWorkspaceData(summary)
  const setupSteps = useMemo(() => dashboardSetupSteps(businessType), [businessType])

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

      <DashboardSavedReports />

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
                  <p>{summary.overdueTasksCount} overdue</p>
                  <p className="field-hint">Review anything that slipped before scheduling new work.</p>
                </div>
                <Button className="button-secondary" type="button" onClick={() => navigate('/tasks?due=overdue')}>Review overdue</Button>
              </article>
              <article className="decision-row" role="listitem">
                <div>
                  <p>{summary.dueSoonTasksCount} due within 24 hours</p>
                  <p className="field-hint">Close or reschedule work that needs attention next.</p>
                </div>
                <Button type="button" onClick={() => navigate('/tasks?due=dueSoon')}>Review due soon</Button>
              </article>
              <article className="decision-row" role="listitem">
                <div>
                  <p>{summary.upcomingTasksCount} later</p>
                  <p className="field-hint">Protect the next few follow-ups before they become urgent.</p>
                </div>
                <Button className="button-secondary" type="button" onClick={() => navigate('/tasks?due=upcoming')}>Review upcoming</Button>
              </article>
            </div>
          </div>
        </Card>

        <DashboardClientReviews summary={summary.clientReviews} isLoading={isLoading} />

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

        <DashboardForecast
          forecast={forecast}
          missingRateCurrencies={summary.missingRateCurrencies}
          forecastPeriod={forecastPeriod}
          setForecastPeriod={setForecastPeriod}
          isLoading={isLoading}
          onApplyPeriod={handleForecastPeriod}
          canManageQuotas={canManageQuotas}
          quotaDrafts={quotaDrafts}
          setQuotaDrafts={setQuotaDrafts}
          savingQuotaUserId={savingQuotaUserId}
          onSaveQuota={handleSaveQuota}
        />

        <Card>
          <div className="card-stack">
            <div>
              <h2>Today</h2>
              <p>See what is live in the pipeline instead of guessing from stale numbers.</p>
            </div>
            {isLoading ? <p className="field-hint">Loading dashboard summary...</p> : null}
            {error ? (
              <InlineError message={error} onRetry={() => loadSummary()} retryLabel="Retry summary" />
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
                      <p>{formatDashboardTimestamp(activity.createdAt)}</p>
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

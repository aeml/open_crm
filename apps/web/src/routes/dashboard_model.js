export const emptyForecast = {
  periodStart: '',
  periodEnd: '',
  currency: 'USD',
  teamQuota: '0',
  wonAmount: '0',
  openPipelineAmount: '0',
  weightedForecastAmount: '0',
  attainmentPct: '0',
  coveragePct: '0',
  missingRateCurrencies: [],
  members: [],
  stages: []
}

export const emptySummary = {
  pipelineValue: '0',
  baseCurrency: 'USD',
  missingRateCurrencies: [],
  openDealsCount: 0,
  wonDealsCount: 0,
  openTasksCount: 0,
  overdueTasksCount: 0,
  dueSoonTasksCount: 0,
  upcomingTasksCount: 0,
  newContactsCount: 0,
  forecast: emptyForecast,
  clientReviews: { total: 0, overdue: 0, dueWithin30Days: 0, later: 0, records: [], semantics: [] },
  recentActivities: []
}

export function formatDashboardMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) return '$0.00'
  const normalizedCurrency = String(currency || 'USD').toUpperCase()
  const safeCurrency = /^[A-Z]{3}$/.test(normalizedCurrency) ? normalizedCurrency : 'USD'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: safeCurrency }).format(amount)
}

export function normalizeDashboardSummary(summary) {
  return {
    ...emptySummary,
    ...(summary || {}),
    forecast: {
      ...emptyForecast,
      ...(summary?.forecast || {}),
      missingRateCurrencies: summary?.forecast?.missingRateCurrencies || [],
      members: summary?.forecast?.members || [],
      stages: summary?.forecast?.stages || []
    },
    missingRateCurrencies: summary?.missingRateCurrencies || [],
    clientReviews: {
      ...emptySummary.clientReviews,
      ...(summary?.clientReviews || {}),
      records: summary?.clientReviews?.records || [],
      semantics: summary?.clientReviews?.semantics || []
    },
    recentActivities: summary?.recentActivities || []
  }
}

export function quotaDraftsFromForecast(forecast) {
  return (forecast?.members || []).reduce((drafts, member) => ({ ...drafts, [member.userId]: member.quotaAmount || '' }), {})
}

export function formatDashboardTimestamp(value) {
  if (!value) return 'Just now'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Just now' : date.toLocaleString()
}

function daysSince(value, now = Date.now()) {
  if (!value) return null
  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime())) return null
  return Math.max(0, Math.floor((now - parsed.getTime()) / (24 * 60 * 60 * 1000)))
}

export function recentlyTouchedRecords(activities) {
  const seen = new Set()
  return (activities || []).filter((activity) => {
    if (activity.entityType !== 'contact' && activity.entityType !== 'company') return false
    const key = `${activity.entityType}:${activity.entityId}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  }).slice(0, 3)
}

export function dashboardLabels(businessType) {
  if (businessType === 'services' || businessType === 'construction-services') {
    return {
      pipelineLabel: 'Open jobs value',
      contactsLabel: 'New contacts',
      openRecordsLabel: 'Open jobs',
      wonRecordsLabel: 'Won jobs',
      recordsLower: 'jobs',
      activityDescription: 'The last real changes across jobs, contacts, clients, and service tasks.'
    }
  }
  return {
    pipelineLabel: 'Open pipeline',
    contactsLabel: 'New contacts',
    openRecordsLabel: 'Open deals',
    wonRecordsLabel: 'Won deals',
    recordsLower: 'deals',
    activityDescription: 'The last real changes across deals, contacts, companies, and tasks.'
  }
}

export function dashboardHeroMetrics(summary, labels) {
  return [
    { label: labels.pipelineLabel, value: formatDashboardMoney(summary.pipelineValue, summary.baseCurrency) },
    { label: 'Due soon', value: `${summary.dueSoonTasksCount} tasks` },
    { label: labels.contactsLabel, value: `${summary.newContactsCount} this week` }
  ]
}

export function dashboardPipelineAttention(summary, labels, now = Date.now()) {
  const latestActivity = (summary.recentActivities || []).find((activity) => activity.entityType === 'deal') || null
  if (summary.openDealsCount <= 0) {
    return {
      title: `No open ${labels.recordsLower}`,
      description: 'The active pipeline is clear. Review recent activity or create the next opportunity when work appears.',
      action: 'Review activity',
      path: '/deals'
    }
  }
  if (!latestActivity) {
    return {
      title: `${summary.openDealsCount} open ${labels.recordsLower} need a touch`,
      description: `No recent ${labels.recordsLower} activity appears in the dashboard feed. Check stage age and next steps before work stalls.`,
      action: `Review ${labels.recordsLower}`,
      path: '/deals'
    }
  }
  const elapsedDays = daysSince(latestActivity.createdAt, now)
  if (elapsedDays !== null && elapsedDays >= 7) {
    return {
      title: `Pipeline has been quiet for ${elapsedDays} days`,
      description: `The latest ${labels.recordsLower.slice(0, -1)} activity was ${latestActivity.summary.toLowerCase()}. Confirm the next move.`,
      action: `Review ${labels.recordsLower}`,
      path: '/deals'
    }
  }
  return {
    title: 'Pipeline touched recently',
    description: `${latestActivity.summary} is the latest pipeline signal. Keep the next task tied to the record.`,
    action: `Open ${labels.recordsLower}`,
    path: '/deals'
  }
}

export function dashboardHasWorkspaceData(summary) {
  const forecast = summary.forecast || emptyForecast
  return Number.parseFloat(summary.pipelineValue || '0') > 0 || Number.parseFloat(forecast.teamQuota || '0') > 0 || summary.openDealsCount > 0 || summary.wonDealsCount > 0 || summary.openTasksCount > 0 || summary.dueSoonTasksCount > 0 || summary.newContactsCount > 0 || summary.recentActivities.length > 0
}

export function dashboardSetupSteps(businessType) {
  const pipelineLabel = businessType === 'services' || businessType === 'construction-services' ? 'Create your first job' : 'Create your first deal'
  return [
    { label: 'Add a contact', description: 'Start with the person you need to follow up with.', action: 'Add contact', path: '/contacts' },
    { label: 'Add a client', description: 'Create the company or individual client behind the work.', action: 'Add client', path: '/companies' },
    { label: pipelineLabel, description: 'Track the opportunity, job, or active revenue conversation.', action: pipelineLabel, path: '/deals' }
  ]
}

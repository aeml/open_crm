import { describe, expect, it } from 'vitest'
import {
  dashboardHasWorkspaceData,
  dashboardHeroMetrics,
  dashboardLabels,
  dashboardPipelineAttention,
  dashboardSetupSteps,
  emptySummary,
  formatDashboardMoney,
  formatDashboardTimestamp,
  normalizeDashboardSummary,
  quotaDraftsFromForecast,
  recentlyTouchedRecords
} from './dashboard_model'

describe('dashboard model', () => {
  it('normalizes partial responses into complete, independent dashboard collections', () => {
    const normalized = normalizeDashboardSummary({
      pipelineValue: '1250.50',
      forecast: { members: [{ userId: 7, quotaAmount: '900' }] },
      clientReviews: { total: 2 }
    })

    expect(normalized.pipelineValue).toBe('1250.50')
    expect(normalized.forecast.members).toEqual([{ userId: 7, quotaAmount: '900' }])
    expect(normalized.forecast.stages).toEqual([])
    expect(normalized.clientReviews).toMatchObject({ total: 2, records: [], semantics: [] })
    expect(normalized.recentActivities).toEqual([])
    expect(quotaDraftsFromForecast(normalized.forecast)).toEqual({ 7: '900' })
  })

  it('formats safe money and timestamp fallbacks without trusting malformed server values', () => {
    expect(formatDashboardMoney('12.5', 'eur')).toBe('€12.50')
    expect(formatDashboardMoney('12.5', 'not-a-currency')).toBe('$12.50')
    expect(formatDashboardMoney('not-a-number', 'USD')).toBe('$0.00')
    expect(formatDashboardTimestamp('')).toBe('Just now')
    expect(formatDashboardTimestamp('not-a-time')).toBe('Just now')
  })

  it('deduplicates the first three contact and client touches while excluding other records', () => {
    const records = recentlyTouchedRecords([
      { entityType: 'deal', entityId: 1 },
      { entityType: 'contact', entityId: 2, summary: 'Newest contact touch' },
      { entityType: 'contact', entityId: 2, summary: 'Older duplicate' },
      { entityType: 'company', entityId: 3 },
      { entityType: 'contact', entityId: 4 },
      { entityType: 'company', entityId: 5 }
    ])
    expect(records.map(({ entityType, entityId }) => `${entityType}:${entityId}`)).toEqual(['contact:2', 'company:3', 'contact:4'])
  })

  it('derives every pipeline attention state from exact open work and activity age', () => {
    const labels = dashboardLabels('general')
    const now = Date.parse('2026-07-23T12:00:00Z')
    expect(dashboardPipelineAttention({ ...emptySummary, openDealsCount: 0 }, labels, now).title).toBe('No open deals')
    expect(dashboardPipelineAttention({ ...emptySummary, openDealsCount: 3 }, labels, now).title).toBe('3 open deals need a touch')

    const stale = dashboardPipelineAttention({
      ...emptySummary,
      openDealsCount: 3,
      recentActivities: [{ entityType: 'deal', createdAt: '2026-07-14T12:00:00Z', summary: 'Proposal updated' }]
    }, labels, now)
    expect(stale.title).toBe('Pipeline has been quiet for 9 days')
    expect(stale.description).toContain('proposal updated')

    const recent = dashboardPipelineAttention({
      ...emptySummary,
      openDealsCount: 3,
      recentActivities: [{ entityType: 'deal', createdAt: '2026-07-23T10:00:00Z', summary: 'Proposal updated' }]
    }, labels, now)
    expect(recent).toMatchObject({ title: 'Pipeline touched recently', action: 'Open deals', path: '/deals' })
  })

  it('keeps adaptive labels, hero values, setup actions, and first-run detection coherent', () => {
    const labels = dashboardLabels('construction-services')
    const summary = normalizeDashboardSummary({ pipelineValue: '2500', baseCurrency: 'USD', dueSoonTasksCount: 2, newContactsCount: 4 })
    expect(labels).toMatchObject({ pipelineLabel: 'Open jobs value', openRecordsLabel: 'Open jobs', recordsLower: 'jobs' })
    expect(dashboardHeroMetrics(summary, labels)).toEqual([
      { label: 'Open jobs value', value: '$2,500.00' },
      { label: 'Due soon', value: '2 tasks' },
      { label: 'New contacts', value: '4 this week' }
    ])
    expect(dashboardSetupSteps('services')[2]).toMatchObject({ label: 'Create your first job', path: '/deals' })
    expect(dashboardHasWorkspaceData(summary)).toBe(true)
    expect(dashboardHasWorkspaceData(normalizeDashboardSummary(null))).toBe(false)
  })
})

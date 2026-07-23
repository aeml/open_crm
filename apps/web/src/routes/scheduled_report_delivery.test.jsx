import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { ScheduledReportDelivery } from './scheduled_report_delivery'

afterEach(() => vi.unstubAllGlobals())

const definition = { id: 7, name: 'Pipeline by stage', isActive: true, visualizationType: 'bar', visualizationContract: 'grouped_bar_v1' }
const users = [
  { id: 1, firstName: 'Demo', lastName: 'Owner', email: 'owner@acme.test', status: 'active' },
  { id: 2, firstName: 'Alex', lastName: 'Admin', email: 'alex@acme.test', status: 'active' }
]

function scheduleOverview() {
  return {
    provider: 'postmark',
    deliveryAvailable: true,
    schedules: [{ id: 3, reportDefinitionId: 7, reportName: definition.name, revision: 2, cadence: 'weekly', weekdayUtc: 1, hourUtc: 13, isActive: true, nextRunAt: '2026-07-27T13:00:00Z', recipients: [{ userId: 1, name: 'Demo Owner', email: 'owner@acme.test', role: 'owner', isActive: true }], createdAt: '2026-07-20T00:00:00Z', updatedAt: '2026-07-21T00:00:00Z' }],
    deliveryRuns: [{ id: 9, scheduleId: 3, reportDefinitionId: 7, reportName: definition.name, scheduleRevision: 2, scheduledFor: '2026-07-21T13:00:00Z', status: 'partial', byteSize: 120, rowCount: 4, recipients: [
      { id: 91, recipientUserId: 1, recipientName: 'Demo Owner', recipientEmail: 'owner@acme.test', status: 'uncertain', attemptCount: 1 },
      { id: 92, recipientUserId: 2, recipientName: 'Alex Admin', recipientEmail: 'alex@acme.test', status: 'failed', attemptCount: 3 }
    ], createdAt: '2026-07-21T13:00:00Z' }]
  }
}

it('saves a revision-safe schedule and makes ambiguous delivery recovery deliberate', async () => {
  const overview = scheduleOverview()
  const fetchMock = vi.fn(async (url, options = {}) => {
    const path = new URL(String(url), 'http://localhost').pathname
    if (path.endsWith('/api/report-schedules')) return { ok: true, json: async () => ({ data: overview }) }
    if (path.endsWith('/api/users')) return { ok: true, json: async () => ({ data: { users } }) }
    if (path.endsWith('/api/report-definitions/7/schedule') && options.method === 'PUT') {
      overview.schedules[0] = { ...overview.schedules[0], ...JSON.parse(options.body), revision: 3, recipients: overview.schedules[0].recipients }
      return { ok: true, json: async () => ({ data: { schedule: overview.schedules[0] } }) }
    }
    if (path.endsWith('/api/report-recipient-deliveries/91/resolve') && options.method === 'POST') {
      overview.deliveryRuns[0].recipients[0].status = 'pending'
      return { ok: true, json: async () => ({ data: { deliveryRun: overview.deliveryRuns[0] } }) }
    }
    return { ok: false, status: 404, json: async () => ({ error: { message: 'Unexpected request.' } }) }
  })
  vi.stubGlobal('fetch', fetchMock)

  render(<ScheduledReportDelivery definitions={[definition]} />)

  expect(await screen.findByRole('heading', { name: /saved-report csv email/i })).toBeInTheDocument()
  expect(screen.getByText(/next occurrence:/i)).toHaveTextContent('UTC')
  fireEvent.change(screen.getByLabelText('Cadence'), { target: { value: 'daily' } })
  fireEvent.change(screen.getByLabelText('Hour (UTC)'), { target: { value: '9' } })
  fireEvent.click(screen.getByLabelText(/alex admin/i))
  fireEvent.click(screen.getByRole('button', { name: /save and enable schedule/i }))

  await waitFor(() => {
    const save = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/report-definitions/7/schedule'))
    expect(JSON.parse(save[1].body)).toEqual({ revision: 2, cadence: 'daily', weekdayUtc: null, hourUtc: 9, recipientUserIds: [1, 2], isActive: true })
  })

  const recipientList = await screen.findByRole('list', { name: /pipeline by stage recipients/i })
  const uncertain = within(recipientList).getByText(/needs review/i).closest('article')
  expect(within(uncertain).getByRole('button', { name: /retry despite duplicate risk/i })).toBeDisabled()
  fireEvent.click(within(uncertain).getByLabelText(/understand retrying/i))
  fireEvent.click(within(uncertain).getByRole('button', { name: /retry despite duplicate risk/i }))

  await waitFor(() => {
    const recovery = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/report-recipient-deliveries/91/resolve'))
    expect(JSON.parse(recovery[1].body)).toEqual({ resolution: 'retry', confirmDuplicateRisk: true })
  })
})

it('shows provider configuration guidance and does not enable saving', async () => {
  vi.stubGlobal('fetch', vi.fn(async (url) => {
    const path = new URL(String(url), 'http://localhost').pathname
    if (path.endsWith('/api/report-schedules')) return { ok: true, json: async () => ({ data: { ...scheduleOverview(), deliveryAvailable: false } }) }
    return { ok: true, json: async () => ({ data: { users } }) }
  }))

  render(<ScheduledReportDelivery definitions={[definition]} />)

  expect(await screen.findByText(/operator must configure postmark/i)).toBeInTheDocument()
  expect(screen.getByRole('button', { name: /save and enable schedule/i })).toBeDisabled()
})

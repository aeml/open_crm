import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AppRouter } from '../app/router'
import { SalesActivityReport } from './sales_activity_report'

afterEach(() => vi.unstubAllGlobals())

describe('sales activity report', () => {
  it('explains partial coverage, renders snapshot metrics, and filters by a retained teammate', async () => {
    const requests = []
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      requests.push(requestURL)
      if (requestURL.pathname.endsWith('/auth/me')) return jsonResponse({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme' }, membership: { role: 'viewer' } } })
      if (requestURL.pathname.endsWith('/api/users')) return jsonResponse({ data: { users: [{ id: 1, firstName: 'Avery', lastName: 'Seller', status: 'active' }, { id: 2, firstName: 'Blake', lastName: 'Seller', status: 'disabled' }] } })
      if (requestURL.pathname.endsWith('/api/reports/sales-activity')) return jsonResponse({ data: {
        coverageStartedAt: '2026-07-19T12:00:00Z', historyComplete: false,
        ownerFilterMeaning: 'Deal metrics use the owner saved on the event; notes and tasks use the teammate who performed the activity.',
        outcomeMeaning: 'A won or lost outcome is a real transition into that outcome; a deal reopened and closed again contributes another outcome.',
        stageConversionMeaning: 'Forward exit rate is forward-or-won stage exits divided by every exit from that stage during the selected period; it is event-based, not a deal-cohort funnel.',
        totals: { dealsCreated: requestURL.searchParams.get('ownerUserId') === '2' ? 1 : 3, stageMoves: 4, dealsWon: 1, dealsLost: 1, winRatePercent: '50.0', notesAdded: 1, tasksCreated: 1, tasksCompleted: 1 },
        owners: [{ userId: 2, userName: 'Blake Seller', email: 'blake@example.test', status: 'disabled', dealsCreated: 1, stageMoves: 1, dealsWon: 0, dealsLost: 1, notesAdded: 1, tasksCreated: 0, tasksCompleted: 0 }],
        stages: [{ pipelineId: 4, pipelineName: 'Sales', stageId: 5, stageName: 'Proposal', entries: 3, exits: 3, forwardExits: 1, wonExits: 1, lostExits: 1, forwardExitRatePercent: '33.3' }],
        dealEvents: [{ id: 9, dealId: 12, dealName: 'Expansion A', eventType: 'stage_changed', actorName: 'Avery Seller', ownerName: 'Avery Seller', fromStageName: 'Proposal', toStageName: 'Won', toStageOutcome: 'won', occurredAt: '2026-07-19T13:00:00Z' }]
      } })
      if (requestURL.pathname.endsWith('/api/data-quality/summary')) return jsonResponse({ data: { reports: [] } })
      if (requestURL.pathname.endsWith('/api/report-definitions')) return jsonResponse({ data: { definitions: [] } })
      return jsonResponse({ data: { unreadCount: 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/reports')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /sales activity/i })).toBeInTheDocument()
    expect(await screen.findByText(/partial event history/i)).toBeInTheDocument()
    expect(screen.getByText('3', { selector: '.metric-value' })).toBeInTheDocument()
    expect(screen.getByText(/sales \/ proposal/i)).toBeInTheDocument()
    expect(screen.getByText(/33.3% forward exits/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Expansion A' })).toHaveAttribute('href', '/deals/12')
    expect(screen.getByText(/event-based, not a deal-cohort funnel/i)).toBeInTheDocument()
    expect(screen.getByText(/reopened and closed again/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Teammate'), { target: { value: '2' } })
    fireEvent.click(screen.getByRole('button', { name: /run report/i }))
    await waitFor(() => expect(requests.some((request) => request.pathname.endsWith('/api/reports/sales-activity') && request.searchParams.get('ownerUserId') === '2')).toBe(true))
    expect(await screen.findByText(/blake seller \(disabled\)/i, { selector: 'option' })).toBeInTheDocument()
  })

  it('keeps the live report visible and retries teammate-filter loading independently', async () => {
    let userAttempts = 0
    const jsonResponse = (payload) => ({ ok: true, status: 200, json: async () => payload })
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/api/users')) {
        userAttempts += 1
        if (userAttempts === 1) return { ok: false, status: 500, json: async () => ({ error: { message: 'Teammates unavailable.' } }) }
        return jsonResponse({ data: { users: [{ id: 7, firstName: 'Recovered', lastName: 'Owner', status: 'active' }] } })
      }
      return jsonResponse({ data: { historyComplete: true, coverageStartedAt: '2026-07-19T12:00:00Z', totals: {}, owners: [], stages: [], dealEvents: [] } })
    }))
    render(<MemoryRouter><SalesActivityReport /></MemoryRouter>)

    expect(await screen.findByText('Teammates unavailable.')).toBeInTheDocument()
    expect(await screen.findByText(/complete event coverage/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry teammates' }))
    expect(await screen.findByRole('option', { name: 'Recovered Owner' })).toBeInTheDocument()
  })
})

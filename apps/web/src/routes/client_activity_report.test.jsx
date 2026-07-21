import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ClientActivityReport } from './client_activity_report'

afterEach(() => vi.unstubAllGlobals())

describe('client activity report', () => {
  it('shows no-activity clients first, links rolled-up sources, and reruns bounded owner filters', async () => {
    const requests = []
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      requests.push(requestURL)
      if (requestURL.pathname.endsWith('/api/users')) return { ok: true, status: 200, json: async () => ({ data: { users: [{ id: 7, firstName: 'Dana', lastName: 'Owner', status: 'disabled' }] } }) }
      return { ok: true, status: 200, json: async () => ({ data: {
        entityType: requestURL.searchParams.get('entityType'),
        fromDate: requestURL.searchParams.get('from'),
        toDate: requestURL.searchParams.get('to'),
        activity: requestURL.searchParams.get('activity'),
        ownerUserId: Number(requestURL.searchParams.get('ownerUserId') || 0),
        generatedAt: '2026-07-21T12:00:00Z',
        count: 2,
        totals: { totalClients: 2, clientsWithActivity: 1, clientsWithoutActivity: 1, qualifyingTouches: 4, notesAdded: 1, tasksCompleted: 1 },
        records: [
          { entityType: 'company', entityId: 8, label: 'Quiet Client', ownerUserName: '', qualifyingTouches: 0, notesAdded: 0, tasksCompleted: 0, activeDays: 0 },
          { entityType: 'company', entityId: 9, label: 'Active Client', ownerUserName: 'Dana Owner', qualifyingTouches: 4, notesAdded: 1, tasksCompleted: 1, activeDays: 3, lastTouchInPeriod: { summary: 'Email received', occurredAt: '2026-07-20T10:00:00Z', recordEntityType: 'contact', recordEntityId: 12, recordLabel: 'Ava Stone' } }
        ],
        semantics: ['This report does not infer historical health changes.']
      } }) }
    }))
    render(<MemoryRouter><ClientActivityReport /></MemoryRouter>)

    const table = await screen.findByRole('table', { name: /client activity from/i })
    const rows = within(table).getAllByRole('row')
    expect(within(rows[1]).getByRole('link', { name: 'Quiet Client' })).toHaveAttribute('href', '/companies/8')
    expect(within(rows[1]).getByText('No qualifying touch in period')).toBeInTheDocument()
    expect(within(rows[2]).getByRole('link', { name: 'Active Client' })).toHaveAttribute('href', '/companies/9')
    expect(within(rows[2]).getByRole('link', { name: 'Email received' })).toHaveAttribute('href', '/contacts/12')
    expect(screen.getByRole('list', { name: 'Client activity totals' })).toHaveTextContent(/without activity1/i)
    expect(screen.getByText(/does not infer historical health changes/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Activity'), { target: { value: 'without_activity' } })
    fireEvent.change(screen.getByLabelText('Current owner'), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: 'Run client activity' }))
    await waitFor(() => expect(requests.some((request) => request.pathname.endsWith('/api/reports/client-activity') && request.searchParams.get('activity') === 'without_activity' && request.searchParams.get('ownerUserId') === '7')).toBe(true))
    expect(screen.getByRole('option', { name: 'Dana Owner (disabled)' })).toBeInTheDocument()
  })

  it('fails closed when a response does not match the requested period', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/api/users')) return { ok: true, status: 200, json: async () => ({ data: { users: [] } }) }
      return { ok: true, status: 200, json: async () => ({ data: { entityType: 'company', fromDate: '2000-01-01', toDate: '2000-01-02', activity: 'all', ownerUserId: 0, totals: {}, records: [], semantics: [] } }) }
    }))
    render(<MemoryRouter><ClientActivityReport /></MemoryRouter>)
    expect(await screen.findByText('The client activity report returned a different filter window.')).toBeInTheDocument()
    expect(screen.queryByRole('table', { name: /client activity from/i })).not.toBeInTheDocument()
  })
})

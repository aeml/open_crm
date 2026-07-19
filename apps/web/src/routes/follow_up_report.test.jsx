import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { FollowUpReport } from './follow_up_report'

afterEach(() => vi.unstubAllGlobals())

describe('follow-up report', () => {
  it('shows no-touch records and reruns for a disabled owner', async () => {
    const requests = []
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      requests.push(requestURL)
      if (requestURL.pathname.endsWith('/api/users')) return { ok: true, json: async () => ({ data: { users: [{ id: 7, firstName: 'Dana', lastName: 'Owner', status: 'disabled' }] } }) }
      return { ok: true, json: async () => ({ data: {
        entityType: requestURL.searchParams.get('entityType'), staleDays: Number(requestURL.searchParams.get('staleDays')), generatedAt: '2026-07-19T12:00:00Z', count: 1,
        records: [{ entityType: 'contact', entityId: 12, label: 'Ava Stone', ownerUserName: 'Dana Owner', createdAt: '2026-01-01T00:00:00Z', referenceAt: '2026-01-01T00:00:00Z', daysSinceReference: 199 }],
        semantics: ['A record with no touch uses its creation time.']
      } }) }
    }))
    render(<MemoryRouter><FollowUpReport /></MemoryRouter>)

    expect(await screen.findByRole('link', { name: 'Ava Stone' })).toHaveAttribute('href', '/contacts/12')
    expect(screen.getByText(/No qualifying touch · created/i)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('No touch for'), { target: { value: '60' } })
    fireEvent.change(screen.getByLabelText('Owner'), { target: { value: '7' } })
    fireEvent.click(screen.getByRole('button', { name: /run report/i }))
    await waitFor(() => expect(requests.some((request) => request.searchParams.get('staleDays') === '60' && request.searchParams.get('ownerUserId') === '7')).toBe(true))
    expect(screen.getByRole('option', { name: 'Dana Owner (disabled)' })).toBeInTheDocument()
  })

  it('attributes a client touch to the linked contact that produced it', async () => {
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/api/users')) return { ok: true, json: async () => ({ data: { users: [] } }) }
      return { ok: true, json: async () => ({ data: {
        entityType: 'company', staleDays: 30, generatedAt: '2026-07-19T12:00:00Z', count: 1,
        records: [{ entityType: 'company', entityId: 8, label: 'Acme', ownerUserName: '', createdAt: '2026-01-01T00:00:00Z', daysSinceReference: 45, lastTouch: { sourceType: 'note', sourceId: 3, action: 'note.created', summary: 'Note added', occurredAt: '2026-06-04T00:00:00Z', recordEntityType: 'contact', recordEntityId: 12, recordLabel: 'Ava Stone' } }], semantics: []
      } }) }
	}))
	render(<MemoryRouter><FollowUpReport /></MemoryRouter>)
	await screen.findByRole('button', { name: /run report/i })
	fireEvent.change(screen.getByLabelText('Record type'), { target: { value: 'company' } })
    fireEvent.click(screen.getByRole('button', { name: /run report/i }))

    expect(await screen.findByRole('link', { name: 'Acme' })).toHaveAttribute('href', '/companies/8')
    expect(screen.getByRole('link', { name: 'Ava Stone' })).toHaveAttribute('href', '/contacts/12')
    expect(screen.getByText(/Source:/i)).toBeInTheDocument()
  })
})

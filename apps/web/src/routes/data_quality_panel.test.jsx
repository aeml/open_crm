import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => vi.unstubAllGlobals())

describe('data quality reports', () => {
  it('shows explainable actionable issues and reruns the stale threshold', async () => {
    const jsonResponse = (payload) => ({ ok: true, json: async () => payload })
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.pathname.endsWith('/auth/me')) return jsonResponse({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme', businessType: 'services' }, membership: { role: 'viewer' } } })
      if (requestURL.pathname.endsWith('/api/data-quality/summary')) {
        const days = Number(requestURL.searchParams.get('staleDays'))
        return jsonResponse({ data: { businessType: 'services', staleDays: days, generatedAt: '2026-07-19T12:00:00Z', reports: [
          { key: 'missing_contact_details', title: 'Contacts without contact details', description: 'Active contacts with neither an email address nor a phone number.', count: 1, records: [{ entityType: 'contact', entityId: 12, label: 'Ava Stone', detail: 'Neither email nor phone is set', updatedAt: '2026-07-01T12:00:00Z' }] },
          { key: 'stale_deals', title: 'Stale open deals', description: `Open deals not updated in the last ${days} days.`, count: 0, records: [] }
        ] } })
      }
      if (requestURL.pathname.endsWith('/api/report-definitions')) return jsonResponse({ data: { definitions: [] } })
      return jsonResponse({ data: { unreadCount: 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/reports')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /contacts without contact details · 1/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Ava Stone' })).toHaveAttribute('href', '/contacts/12')
    expect(screen.getByText(/generated/i)).toBeInTheDocument()
    expect(screen.getByText(/neither email nor phone/i)).toBeInTheDocument()
    expect(screen.getByText('No matching issues.')).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Stale-deal window'), { target: { value: '60' } })
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringContaining('staleDays=60'), expect.anything()))
    expect(await screen.findByText(/last 60 days/i)).toBeInTheDocument()
  })
})

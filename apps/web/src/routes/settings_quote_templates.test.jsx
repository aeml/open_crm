import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => vi.unstubAllGlobals())

function response(payload) {
  return { ok: true, json: async () => payload }
}

describe('settings quote templates route', () => {
  it('creates a revisioned template, enables policy, and decides another admin request', async () => {
    const existing = {
      id: 3, name: 'Standard terms', terms: 'Net 30.', defaultValidityDays: 30,
      deliverySubjectTemplate: 'Quote {{quote_number}}', deliveryMessageTemplate: 'Hi {{recipient_name}}',
      requestSignature: false, requiresApproval: false, isActive: true, revision: 1,
      updatedByUserName: 'Demo Owner', updatedAt: '2026-07-21T09:00:00Z'
    }
    const created = { ...existing, id: 4, name: 'Services MSA', requiresApproval: true }
    let storedTemplates = [existing]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return response({ data: { user: { id: 1, email: 'owner@acme.test' }, organization: { id: 1, name: 'Acme' }, membership: { role: 'owner' } } })
      if (path.endsWith('/api/notifications/unread-count')) return response({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/quote-templates/policy') && method === 'PUT') return response({ data: { policy: { approvalRequired: true, activeApprovers: 2, updatedByUserName: 'Demo Owner' } } })
      if (path.endsWith('/api/quote-templates/policy')) return response({ data: { policy: { approvalRequired: false, activeApprovers: 2 } } })
      if (path.endsWith('/api/quote-templates/merge-tokens')) return response({ data: { tokens: ['{{quote_number}}', '{{recipient_name}}'] } })
      if (path.endsWith('/api/quote-templates') && method === 'POST') {
        storedTemplates = [created, ...storedTemplates]
        return response({ data: { template: created } })
      }
      if (path.endsWith('/api/quote-templates')) return response({ data: { templates: storedTemplates, meta: { page: 1, pageSize: 50, total: storedTemplates.length } } })
      if (path.endsWith('/api/deal-quote-approvals')) return response({ data: { approvals: [{
        approvalId: 8, dealId: 12, dealName: 'Acme renewal', quoteId: 71, quoteNumber: 'Q-12-V1',
        recipientName: 'Ava Stone', currency: 'USD', total: '308.00', pdfSha256: 'a'.repeat(64),
        requestedByUserId: 2, requestedByUserName: 'Alex Admin', requestedAt: '2026-07-21T10:00:00Z'
      }] } })
      if (path.endsWith('/api/deals/12/quotes/71/approval') && method === 'POST') return response({ data: { quote: { id: 71, approval: { status: 'rejected' } } } })
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/quote-templates')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Quote templates' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: 'Standard terms' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /Q-12-V1 · Acme renewal/i })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Template name'), { target: { value: 'Services MSA' } })
    fireEvent.click(screen.getByRole('checkbox', { name: /require independent approval for this template/i }))
    fireEvent.click(screen.getByRole('button', { name: 'Create template' }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/quote-templates$/), expect.objectContaining({ method: 'POST' })))
    expect(await screen.findByRole('heading', { name: 'Services MSA' })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /require approval for every quote/i }))
    await waitFor(() => expect(screen.getByText(/independent approval required workspace-wide/i)).toBeInTheDocument())

    fireEvent.change(screen.getByLabelText(/decision note for Q-12-V1/i), { target: { value: 'Revise the payment schedule.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Reject with note' }))
    await waitFor(() => expect(screen.queryByRole('heading', { name: /Q-12-V1 · Acme renewal/i })).not.toBeInTheDocument())
    const decisionCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/deals/12/quotes/71/approval'))
    expect(decisionCall[1].headers['Idempotency-Key']).toMatch(/^quote-approval-/)
    expect(JSON.parse(decisionCall[1].body)).toEqual({ decision: 'rejected', note: 'Revise the payment schedule.' })
  })

  it('pages and searches retained quote template history without hiding active selection capacity', async () => {
    const templates = Array.from({ length: 51 }, (_, index) => ({
      id: index + 1,
      name: `Retained terms ${String(index + 1).padStart(3, '0')}`,
      terms: 'Net 30.',
      defaultValidityDays: 30,
      deliverySubjectTemplate: 'Quote {{quote_number}}',
      deliveryMessageTemplate: 'Hi {{recipient_name}}',
      requestSignature: false,
      requiresApproval: false,
      isActive: false,
      revision: 1,
      updatedByUserName: 'Demo Owner',
      updatedAt: '2026-07-21T09:00:00Z'
    }))
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      if (path.endsWith('/auth/me')) return response({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme' }, membership: { role: 'owner' } } })
      if (path.endsWith('/api/notifications/unread-count')) return response({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/quote-templates/policy')) return response({ data: { policy: { approvalRequired: false, activeApprovers: 2 } } })
      if (path.endsWith('/api/quote-templates/merge-tokens')) return response({ data: { tokens: [] } })
      if (path.endsWith('/api/deal-quote-approvals')) return response({ data: { approvals: [] } })
      if (path.endsWith('/api/quote-templates')) {
        const search = requestURL.searchParams.get('q') || ''
        const page = Number(requestURL.searchParams.get('page') || 1)
        const filtered = templates.filter((template) => template.name.toLowerCase().includes(search.toLowerCase()))
        const start = (page - 1) * 50
        return response({ data: { templates: filtered.slice(start, start + 50), meta: { page, pageSize: 50, total: filtered.length } } })
      }
      throw new Error(`Unexpected fetch: ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/quote-templates')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Retained terms 001' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Retained terms 051' })).not.toBeInTheDocument()
    expect(screen.getByText(/Showing 50 of 51 quote templates/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    expect(await screen.findByRole('heading', { name: 'Retained terms 051' })).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText('Search quote templates'), { target: { value: 'Retained terms 051' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply search' }))
    expect(await screen.findByText(/Showing 1 of 1 quote templates matching/)).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/quote-templates\?q=Retained\+terms\+051&page=1&pageSize=50/), expect.any(Object))
    expect(screen.getByText(/Up to 100 templates may be active/)).toBeInTheDocument()
  })
})

import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { ClientHealthReport } from './client_health_report'

afterEach(() => vi.unstubAllGlobals())

describe('client health report', () => {
  it('filters explainable client signals and opens the selected account', async () => {
    const requests = []
    const onOpen = vi.fn()
    vi.stubGlobal('fetch', vi.fn(async (url) => {
      requests.push(new URL(String(url), 'http://localhost'))
      return { ok: true, json: async () => ({ data: {
        entityType: 'company', status: 'all', count: 1,
        totals: { total: 3, healthy: 1, watch: 1, needsAttention: 1 },
        records: [{ entityType: 'company', entityId: 9, label: 'At-risk account', healthStatus: 'needs_attention', healthLabel: 'Needs attention', healthReasons: ['No qualifying touch for 45 days', '1 overdue open task'], openTaskCount: 2, ownerUserName: 'Alex Admin' }],
        semantics: ['Open tasks without a due date are counted but do not change health.']
      } }) }
    }))

    render(<ClientHealthReport onOpen={onOpen} owners={[{ id: 4, firstName: 'Dana', lastName: 'Disabled', status: 'disabled' }]} />)

    expect(await screen.findByText('Needs attention: 1')).toBeInTheDocument()
    expect(await screen.findByRole('option', { name: 'Dana Disabled (disabled)' })).toBeInTheDocument()
    expect(screen.getByText('No qualifying touch for 45 days')).toBeInTheDocument()
    expect(screen.getByText('1 overdue open task')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'At-risk account' }))
    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ entityType: 'company', entityId: 9 }))

    fireEvent.change(screen.getByLabelText('Health'), { target: { value: 'watch' } })
    fireEvent.change(screen.getByLabelText('Stale after'), { target: { value: '60' } })
    fireEvent.change(screen.getByLabelText('Owner'), { target: { value: '4' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply health filters' }))
    await waitFor(() => expect(requests.some((request) => request.searchParams.get('status') === 'watch' && request.searchParams.get('staleDays') === '60' && request.searchParams.get('ownerUserId') === '4')).toBe(true))
    fireEvent.click(screen.getByText('How client health is calculated'))
    expect(screen.getByText(/without a due date/i)).toBeInTheDocument()
  })

  it('reuses scoped health segments without mixing ordinary client-list views', async () => {
    const requests = []
    const fetchMock = vi.fn(async (url, options = {}) => {
      const request = new URL(String(url), 'http://localhost')
      requests.push(request)
      if (request.pathname === '/api/saved-views') {
        if (options.method === 'POST') {
          return { ok: true, json: async () => ({ data: { view: { id: 13, name: 'Renewal watch', filters: JSON.parse(options.body).filters } } }) }
        }
        return { ok: true, json: async () => ({ data: { views: [
          { id: 11, name: 'Needs attention', filters: { savedViewScope: 'client-health', entityType: 'contact', status: 'needs_attention', staleDays: '60', ownerUserId: '4' } },
          { id: 13, name: 'Renewal watch', filters: { savedViewScope: 'client-health', entityType: 'contact', status: 'needs_attention', staleDays: '60', ownerUserId: '4' } },
          { id: 12, name: 'Ordinary client list', filters: { q: 'Acme' } }
        ] } }) }
      }
      return { ok: true, json: async () => ({ data: { entityType: request.searchParams.get('entityType') || 'company', status: request.searchParams.get('status') || 'all', count: 0, totals: { total: 0, healthy: 0, watch: 0, needsAttention: 0 }, records: [], semantics: [] } }) }
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<ClientHealthReport onOpen={vi.fn()} owners={[{ id: 4, firstName: 'Ari', lastName: 'Owner' }]} />)

    await screen.findByText(/0 of 0 organization clients/i)
    fireEvent.click(screen.getByRole('button', { name: 'Load segments' }))
    expect(await screen.findByRole('option', { name: 'Needs attention' })).toBeInTheDocument()
    expect(screen.queryByRole('option', { name: 'Ordinary client list' })).not.toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Saved segments'), { target: { value: '11' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply segment' }))

    await waitFor(() => expect(requests.some((request) => request.pathname.endsWith('/client-health') && request.searchParams.get('entityType') === 'contact' && request.searchParams.get('status') === 'needs_attention' && request.searchParams.get('staleDays') === '60' && request.searchParams.get('ownerUserId') === '4')).toBe(true))
    expect(screen.getByLabelText('Client type')).toHaveValue('contact')
    expect(screen.getByLabelText('Health')).toHaveValue('needs_attention')

    fireEvent.change(screen.getByLabelText('Save current segment as'), { target: { value: 'Renewal watch' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save segment' }))
    await waitFor(() => expect(fetchMock.mock.calls.some(([url, options]) => {
      if (!String(url).endsWith('/api/saved-views') || options?.method !== 'POST') return false
      const body = JSON.parse(options.body)
      return body.entityType === 'companies' &&
        body.isDefault === false &&
        body.filters?.savedViewScope === 'client-health' &&
        body.filters?.entityType === 'contact' &&
        body.filters?.status === 'needs_attention' &&
        body.filters?.staleDays === '60' &&
        body.filters?.ownerUserId === '4'
    })).toBe(true))
  })
})

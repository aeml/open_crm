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
})

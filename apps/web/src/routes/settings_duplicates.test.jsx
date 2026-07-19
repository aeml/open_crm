import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function candidateRecord(id, label, phone, related = {}) {
  return {
    id,
    label,
    updatedAt: `2026-07-19T10:3${id}:00Z`,
    related,
    fields: [
      { key: 'firstName', label: 'First name', value: 'Ava', displayValue: 'Ava', selectable: true },
      { key: 'email', label: 'Email', value: `ava-${id}@example.test`, displayValue: `ava-${id}@example.test`, selectable: true },
      { key: 'phone', label: 'Phone', value: phone, displayValue: phone || 'Not set', selectable: true },
      { key: 'isClient', label: 'Client status', value: id === 1 ? 'true' : 'false', displayValue: id === 1 ? 'true' : 'false', selectable: false },
      { key: 'custom:vip', label: 'VIP', value: id === 1 ? false : null, displayValue: id === 1 ? 'No' : 'Not set', selectable: true },
      { key: 'custom:annual_revenue', label: 'Annual revenue', value: id === 1 ? 0 : null, displayValue: id === 1 ? '0' : 'Not set', selectable: true }
    ]
  }
}

describe('settings duplicate review route', () => {
  it('keeps a stable idempotency key across a failed retry and resolves fields explicitly', async () => {
    const source = candidateRecord(1, 'Ava Duplicate', '+1 202 555 0199', { notes: 2 })
    const target = candidateRecord(2, 'Ava Primary', '', { deals: 1 })
    const candidate = { entityType: 'contact', reasons: ['matching email'], first: source, second: target }
    const operation = {
      id: 9,
      entityType: 'contact',
      sourceEntityId: 1,
      sourceLabel: source.label,
      targetEntityId: 2,
      targetLabel: target.label,
      sourceFields: ['phone'],
      relationshipCounts: { notes: 2 },
      createdAt: '2026-07-19T11:00:00Z'
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 7 }, organization: { id: 42, name: 'Acme' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { candidates: [candidate], recentMerges: [] } }) })
      .mockResolvedValueOnce({ ok: false, status: 503, json: async () => ({ error: { message: 'Temporary database failure' } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { operation } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { candidates: [], recentMerges: [operation] } }) })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))
    window.history.pushState({}, '', '/settings/data-quality')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /duplicate review/i })).toBeInTheDocument()
    expect(await screen.findByText(/matching email/i)).toBeInTheDocument()
    expect(screen.getByText(/2 notes/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: `Keep ${target.label} (ava-2@example.test)` }))
    expect(screen.getByRole('heading', { name: `Merge into ${target.label}` })).toBeInTheDocument()
    expect(screen.getByLabelText('+1 202 555 0199')).toBeChecked()
    expect(screen.getByLabelText('No')).toBeChecked()
    expect(screen.getByLabelText('0')).toBeChecked()

    const mergeButton = screen.getByRole('button', { name: `Merge and archive ${source.label}` })
    fireEvent.click(mergeButton)
    expect(await screen.findByText(/temporary database failure/i)).toBeInTheDocument()
    fireEvent.click(mergeButton)
    expect(await screen.findByText(/merge complete/i)).toBeInTheDocument()

    const mergeCalls = fetchMock.mock.calls.filter(([targetURL]) => String(targetURL).endsWith('/api/data-operations/duplicates/merge'))
    expect(mergeCalls).toHaveLength(2)
    const firstBody = JSON.parse(mergeCalls[0][1].body)
    const secondBody = JSON.parse(mergeCalls[1][1].body)
    expect(firstBody.idempotencyKey).toMatch(/^merge-/)
    expect(secondBody.idempotencyKey).toBe(firstBody.idempotencyKey)
    expect(firstBody).toMatchObject({ entityType: 'contact', sourceEntityId: 1, targetEntityId: 2, sourceFields: ['phone', 'custom:vip', 'custom:annual_revenue'] })
    expect(await screen.findByText(/no likely duplicate contacts found/i)).toBeInTheDocument()
    expect(vi.mocked(globalThis.confirm)).toHaveBeenCalledWith(expect.stringMatching(/cannot be automatically undone/i))
  })

  it('does not load or expose duplicate controls to non-admin members', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 8 }, organization: { id: 42, name: 'Acme' }, membership: { role: 'member' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/data-quality')
    render(<AppRouter />)

    expect(await screen.findByText(/admin access required/i)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: /duplicate review/i })).not.toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  })
})

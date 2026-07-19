import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { BulkActions, bulkStatusOptions } from './bulk_actions'

afterEach(() => {
  vi.unstubAllGlobals()
})

function Harness({ onChanged = vi.fn(), initialIds = [1, 2] }) {
  const [selectedIds, setSelectedIds] = useState(initialIds)
  return <BulkActions entityType="contact" selectedIds={selectedIds} visibleIds={[1, 2]} onSelectionChange={setSelectedIds} onChanged={onChanged} statuses={bulkStatusOptions.contact} userOptions={[{ id: 9, firstName: 'Mina', lastName: 'Owner', membershipStatus: 'active' }, { id: 10, firstName: 'Disabled', lastName: 'User', membershipStatus: 'disabled' }]} />
}

describe('bulk actions', () => {
  it('keeps one idempotency key while a failed change is retried', async () => {
    const onChanged = vi.fn(async () => {})
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: false, status: 500, json: async () => ({ error: { message: 'Temporary failure' } }) })
      .mockResolvedValueOnce({ ok: true, status: 201, json: async () => ({ data: { operation: { id: 7, entityType: 'contact', action: 'reassign', targetUserName: 'Mina Owner', targetCount: 2, changedCount: 2, status: 'completed' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { operations: [] } }) })
    vi.stubGlobal('fetch', fetchMock)
    render(<Harness onChanged={onChanged} />)

    fireEvent.change(screen.getByLabelText(/new owner/i), { target: { value: '9' } })
    fireEvent.click(screen.getByRole('button', { name: /apply to 2 selected/i }))
    expect(await screen.findByText(/temporary failure/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /apply to 2 selected/i }))
    expect(await screen.findByText(/assign to mina owner: 2 of 2 changed/i)).toBeInTheDocument()

    const postCalls = fetchMock.mock.calls.filter(([, request]) => request?.method === 'POST')
    const firstBody = JSON.parse(postCalls[0][1].body)
    const secondBody = JSON.parse(postCalls[1][1].body)
    expect(firstBody).toMatchObject({ entityType: 'contact', action: 'reassign', targetUserId: 9, entityIds: [1, 2] })
    expect(firstBody.idempotencyKey).toMatch(/^bulk-/)
    expect(secondBody.idempotencyKey).toBe(firstBody.idempotencyKey)
    expect(onChanged).toHaveBeenCalledOnce()
    expect(screen.queryByText(/disabled user/i)).not.toBeInTheDocument()
  })

  it('surfaces persistent history and safely requests rollback', async () => {
    const onChanged = vi.fn(async () => {})
    const operation = { id: 12, entityType: 'contact', action: 'archive', status: 'completed', targetCount: 2, changedCount: 2, createdAt: '2026-07-19T10:00:00Z' }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { operations: [operation] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { operation: { ...operation, status: 'partially_rolled_back', rolledBackCount: 1, rollbackSkippedCount: 1 } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { operations: [{ ...operation, status: 'partially_rolled_back', rolledBackCount: 1, rollbackSkippedCount: 1 }] } }) })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))
    render(<Harness onChanged={onChanged} initialIds={[]} />)

    fireEvent.click(await screen.findByText(/recent bulk changes/i))
    fireEvent.click(await screen.findByRole('button', { name: /^undo$/i }))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/data-operations\/bulk\/12\/rollback$/), expect.objectContaining({ method: 'POST' })))
    expect(await screen.findByText(/undo complete: 1 restored, 1 kept because they changed later/i)).toBeInTheDocument()
    expect(onChanged).toHaveBeenCalledOnce()
  })
})

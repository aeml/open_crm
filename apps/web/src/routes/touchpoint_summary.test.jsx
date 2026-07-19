import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TouchpointSummary } from './touchpoint_summary'

afterEach(() => vi.unstubAllGlobals())

describe('touchpoint summary', () => {
  it('shows a traceable linked-contact touch and explains inference', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ data: {
      entityType: 'company', entityId: 8, label: 'Acme', createdAt: '2026-01-01T00:00:00Z', staleDays: 30, isStale: true,
      lastTouch: { sourceType: 'email', sourceId: 4, action: 'email.received', summary: 'Email received', occurredAt: '2026-05-01T12:00:00Z', recordEntityType: 'contact', recordEntityId: 12, recordLabel: 'Ava Stone' },
      recent: [{ sourceType: 'email', sourceId: 4, action: 'email.received', summary: 'Email received', occurredAt: '2026-05-01T12:00:00Z', recordEntityType: 'contact', recordEntityId: 12, recordLabel: 'Ava Stone' }],
      semantics: ['Routine record edits are not touches.']
    } }) })))
    render(<MemoryRouter><TouchpointSummary entityType="company" entityId={8} /></MemoryRouter>)

    expect(await screen.findByText(/needs follow-up · 30\+ days/i)).toBeInTheDocument()
    expect(screen.getAllByRole('link', { name: 'Ava Stone' })[0]).toHaveAttribute('href', '/contacts/12')
    expect(screen.getByText('email.received')).toBeInTheDocument()
    fireEvent.click(screen.getByText('How touchpoints are calculated'))
    expect(screen.getByText('Routine record edits are not touches.')).toBeInTheDocument()
  })

  it('makes the no-touch creation fallback explicit and retries safely', async () => {
    let attempts = 0
    vi.stubGlobal('fetch', vi.fn(async () => {
      attempts += 1
      if (attempts === 1) return { ok: false, status: 500, json: async () => ({ error: { message: 'History unavailable.' } }) }
      return { ok: true, json: async () => ({ data: { entityType: 'contact', entityId: 3, createdAt: '2026-07-01T00:00:00Z', staleDays: 30, isStale: false, recent: [], semantics: [] } }) }
    }))
    render(<MemoryRouter><TouchpointSummary entityType="contact" entityId={3} /></MemoryRouter>)

    expect(await screen.findByText('History unavailable.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry follow-up history' }))
    expect(await screen.findByText(/No qualifying touch yet/i)).toBeInTheDocument()
    expect(screen.getByText('No touchpoints recorded.')).toBeInTheDocument()
  })
})

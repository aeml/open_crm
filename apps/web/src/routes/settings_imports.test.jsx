import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('settings imports route', () => {
  it('maps, dry-runs, imports, reports errors, and safely rolls back a batch', async () => {
    const completedBatch = {
      id: 12,
      entityType: 'contacts',
      originalFilename: 'clients.csv',
      status: 'completed_with_errors',
      totalRows: 2,
      processedRows: 2,
      successRows: 1,
      errorRows: 1,
      rolledBackRows: 0,
      rollbackSkippedRows: 0,
      createdByName: 'Demo Owner',
      createdAt: '2026-07-19T12:00:00Z',
      jobStatus: 'succeeded',
      jobAttempts: 1,
      jobMaxAttempts: 3
    }
    const queuedBatch = { ...completedBatch, status: 'processing', processedRows: 0, successRows: 0, errorRows: 0, jobStatus: 'pending', jobAttempts: 0 }
    const rolledBackBatch = { ...completedBatch, status: 'rolled_back', rolledBackRows: 1 }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 42, name: 'Acme' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { batches: [] } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            entityType: 'contacts',
            sourceColumns: ['Given Name', 'Family Name', 'Email Address'],
            fields: [
              { key: 'first_name', label: 'First name', required: true },
              { key: 'last_name', label: 'Last name', required: true },
              { key: 'email', label: 'Email', required: false }
            ],
            mapping: { first_name: 'Given Name', last_name: 'Family Name', email: 'Email Address' },
            mappingErrors: [],
            summary: { totalRows: 2, validRows: 1, errorRows: 1 },
            rows: [
              { rowNumber: 2, values: { first_name: 'Ava', last_name: 'Stone', email: 'ava@example.test' }, errors: [], warnings: [] },
              { rowNumber: 3, values: { first_name: 'Missing', last_name: '', email: '' }, errors: [{ column: 'last_name', message: 'Last name is required' }], warnings: [] }
            ]
          }
        })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { batch: queuedBatch } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { batches: [completedBatch] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { batch: rolledBackBatch } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { batches: [rolledBackBatch] } }) })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))
    window.history.pushState({}, '', '/settings/imports')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /import crm data/i })).toBeInTheDocument()
    const file = new File(['Given Name,Family Name,Email Address\nAva,Stone,ava@example.test\nMissing,,\n'], 'clients.csv', { type: 'text/csv' })
    fireEvent.change(screen.getByLabelText(/csv file/i), { target: { files: [file] } })
    fireEvent.click(screen.getByRole('button', { name: /preview and map/i }))

    expect(await screen.findByText(/1 valid/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/first name column/i)).toHaveValue('Given Name')
    expect(screen.getByText(/last name is required/i)).toBeInTheDocument()
    const previewCall = fetchMock.mock.calls.find(([target]) => String(target).endsWith('/api/imports/preview'))
    expect(previewCall?.[1]?.body).toBeInstanceOf(FormData)
    expect(previewCall[1].body.get('file').name).toBe('clients.csv')

    fireEvent.click(screen.getByRole('button', { name: /import valid rows/i }))
    expect(await screen.findByText(/import queued: 0 \/ 2 processed/i)).toBeInTheDocument()
    expect(await screen.findByText(/clients.csv · completed with errors/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /download errors/i })).toHaveAttribute('href', expect.stringMatching(/\/api\/imports\/12\/errors\.csv$/))
    const importCall = fetchMock.mock.calls.find(([target]) => String(target).endsWith('/api/imports'))
    expect(importCall?.[1]?.body.get('idempotencyKey')).toMatch(/^import-/)
    expect(importCall?.[1]?.body.get('mapping')).toContain('Given Name')

    fireEvent.click(screen.getByRole('button', { name: /roll back import/i }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/imports\/12\/rollback$/), expect.objectContaining({ method: 'POST' }))
    })
    expect(await screen.findByText(/rollback finished: 1 archived, 0 changed records left active/i)).toBeInTheDocument()
    expect(await screen.findByText(/clients.csv · rolled back/i)).toBeInTheDocument()
  })

  it('exposes dead durable work and safe partial rollback recovery', async () => {
    const failedBatch = {
      id: 27,
      entityType: 'contacts',
      originalFilename: 'interrupted.csv',
      status: 'failed',
      totalRows: 100,
      processedRows: 50,
      successRows: 49,
      errorRows: 1,
      rolledBackRows: 0,
      rollbackSkippedRows: 0,
      createdByName: 'Demo Owner',
      createdAt: '2026-07-19T12:00:00Z',
      failureMessage: 'The retained import source is unavailable; upload a new import or roll back partial results.',
      jobStatus: 'dead',
      jobAttempts: 3,
      jobMaxAttempts: 3
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 42, name: 'Acme' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { batches: [failedBatch] } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/imports')
    render(<AppRouter />)

    expect(await screen.findByText(/interrupted.csv · failed/i)).toBeInTheDocument()
    expect(screen.getByText(/worker: dead · attempt 3\/3/i)).toBeInTheDocument()
    expect(screen.getByText(/retained import source is unavailable/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /review in operations/i })).toHaveAttribute('href', '/settings/operations')
    expect(screen.getByRole('button', { name: /roll back import/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /resume with selected file/i })).not.toBeInTheDocument()
  })

  it('does not expose import controls to a viewer', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 4 }, organization: { id: 42, name: 'Acme' }, membership: { role: 'viewer' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/imports')
    render(<AppRouter />)

    expect(await screen.findByText(/admin access required/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /preview and map/i })).not.toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })
})

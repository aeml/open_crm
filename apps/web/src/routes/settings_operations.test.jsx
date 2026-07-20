import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('settings operations route', () => {
  it('shows job health and lets an admin replay a safe dead job', async () => {
    const jobsResponse = {
      ok: true,
      json: async () => ({
        data: {
          jobs: [{ id: 7, type: 'mailbox.sync', status: 'dead', attempts: 5, maxAttempts: 5, lastError: 'Provider unavailable', createdAt: '2026-07-19T12:00:00Z' }],
          stats: { dead: 1, running: 0, retryable: 0, pending: 0, succeeded: 3 }
        }
      })
    }
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { user: { id: 1, email: 'owner@acme.test' }, organization: { id: 1, name: 'Acme, Inc.' }, membership: { role: 'owner' } } })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce(jobsResponse)
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { job: { id: 7, type: 'mailbox.sync', status: 'pending' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { jobs: [], stats: { dead: 0, pending: 1 } } }) })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/operations')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /background operations/i })).toBeInTheDocument()
    expect(await screen.findByText(/provider unavailable/i)).toBeInTheDocument()
    expect(screen.getByText(/1 need attention/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /replay job/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/admin\/background-jobs\/7\/replay$/), expect.objectContaining({ method: 'POST' }))
    })
    expect(await screen.findByText(/mailbox sync queued for replay/i)).toBeInTheDocument()
  })

  it('requires an explicit decision when an SMTP delivery is uncertain', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme' }, membership: { role: 'admin' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { jobs: [{ id: 8, type: 'email_sequence.send', status: 'dead', attempts: 1, maxAttempts: 5, lastError: 'email sequence delivery outcome is uncertain' }], stats: { dead: 1 } } })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { resolution: { jobId: 8, resolution: 'retry', jobStatus: 'pending' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { jobs: [], stats: { pending: 1 } } }) })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))
    window.history.pushState({}, '', '/settings/operations')
    render(<AppRouter />)

    expect(await screen.findByText(/delivery may already have reached smtp/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /confirm already sent/i })).toBeEnabled()
    fireEvent.click(screen.getByRole('button', { name: /retry email/i }))
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/admin\/background-jobs\/8\/resolve-sequence-delivery$/), expect.objectContaining({ method: 'POST', body: JSON.stringify({ resolution: 'retry' }) }))
    })
    expect(await screen.findByText(/operator-approved retry/i)).toBeInTheDocument()
  })

  it('labels failed billing reconciliation for safe operator replay', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { jobs: [{ id: 9, type: 'billing.reconcile', status: 'dead', attempts: 5, maxAttempts: 5, lastError: 'Stripe request failed' }], stats: { dead: 1 } } })
      })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/operations')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /billing reconciliation · dead/i })).toBeInTheDocument()
    expect(screen.getByText(/re-reads ordered, tenant-matched Stripe state/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /replay job/i })).toBeEnabled()
  })

  it('labels and filters durable billing usage snapshot jobs', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { jobs: [{ id: 11, type: 'billing.usage.snapshot', status: 'dead', attempts: 5, maxAttempts: 5, lastError: 'Usage source unavailable' }], stats: { dead: 1 } } })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { jobs: [], stats: { dead: 0 } } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/operations')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /billing usage snapshot · dead/i })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Job type'), { target: { value: 'billing.usage.snapshot' } })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/type=billing\.usage\.snapshot/), expect.any(Object))
    })
  })

  it('labels and filters portable workspace export recovery jobs', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 1, name: 'Acme' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { jobs: [{ id: 10, type: 'workspace.export.generate', status: 'dead', attempts: 3, maxAttempts: 3, lastError: 'Export coverage must be updated' }], stats: { dead: 1 } } })
      })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { jobs: [], stats: { dead: 0 } } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/operations')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /workspace export · dead/i })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Job type'), { target: { value: 'workspace.export.generate' } })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/type=workspace\.export\.generate/), expect.any(Object))
    })
  })
})

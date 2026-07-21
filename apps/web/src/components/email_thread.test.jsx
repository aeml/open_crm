import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { EmailThread } from './email_thread'

afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('email conversation', () => {
  it('replies with a stable idempotency key and reloads the accepted message', async () => {
    let sent = false
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      const method = options.method || 'GET'
      if (path.endsWith('/api/email-threads/8/reply') && method === 'POST') {
        expect(JSON.parse(options.body)).toEqual({ body: 'We can meet Tuesday.' })
        expect(options.headers['Idempotency-Key']).toMatch(/^email-reply-/)
        sent = true
        return { ok: true, json: async () => ({ data: { reply: { id: 50, status: 'accepted' } } }) }
      }
      if (path.endsWith('/api/email-threads/8')) {
        const messages = [{ id: 8, direction: 'inbound', fromEmail: 'customer@example.test', subject: 'Schedule', body: 'Does Tuesday work?', receivedAt: '2026-07-21T01:00:00Z' }]
        if (sent) messages.push({ id: 9, direction: 'outbound', fromEmail: 'rep@acme.test', toEmail: 'customer@example.test', sentByName: 'Rep Person', subject: 'Re: Schedule', body: 'We can meet Tuesday.', createdAt: '2026-07-21T01:01:00Z' })
        return { ok: true, json: async () => ({ data: { messages, replies: [] } }) }
      }
      throw new Error(`unexpected request ${method} ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<EmailThread messageId={8} canWrite currentUserId={1} canManageReplies={false} />)
    expect(await screen.findByText(/does tuesday work/i)).toBeInTheDocument()
    fireEvent.change(screen.getByRole('textbox', { name: /reply to customer@example.test/i }), { target: { value: 'We can meet Tuesday.' } })
    fireEvent.click(screen.getByRole('button', { name: /^send reply$/i }))

    await waitFor(() => expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/email-threads/8/reply'))).toBe(true))
    expect(await screen.findByText('We can meet Tuesday.')).toBeInTheDocument()
    expect(screen.getByRole('textbox', { name: /reply to customer@example.test/i })).toHaveValue('')
  })

  it('requires explicit confirmation to resolve an uncertain send', async () => {
    const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)
    let resolved = false
    const fetchMock = vi.fn(async (url, options = {}) => {
      const path = new URL(String(url), 'http://localhost').pathname
      if (path.endsWith('/api/email-replies/70/resolve')) {
        expect(JSON.parse(options.body)).toEqual({ resolution: 'retry' })
        resolved = true
        return { ok: true, json: async () => ({ data: { reply: { id: 70, status: 'accepted' } } }) }
      }
      if (path.endsWith('/api/email-threads/8')) {
        return { ok: true, json: async () => ({ data: {
          messages: [{ id: 8, direction: 'inbound', fromEmail: 'customer@example.test', subject: 'Schedule', body: 'Does Tuesday work?', receivedAt: '2026-07-21T01:00:00Z' }],
          replies: resolved ? [] : [{ id: 70, actorUserId: 1, senderEmail: 'rep@acme.test', recipientEmail: 'customer@example.test', subject: 'Re: Schedule', body: 'Possibly sent', status: 'uncertain', lastError: 'Check the Sent folder.' }]
        } }) }
      }
      throw new Error(`unexpected request ${path}`)
    })
    vi.stubGlobal('fetch', fetchMock)

    render(<EmailThread messageId={8} canWrite currentUserId={1} canManageReplies={false} />)
    expect(await screen.findByText(/outcome uncertain/i)).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: /reply to/i })).not.toBeInTheDocument()
    expect(screen.getByText(/resolve your uncertain or in-progress reply/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /retry explicitly/i }))
    expect(confirmSpy).toHaveBeenCalledWith(expect.stringMatching(/can send a duplicate/i))
    await waitFor(() => expect(fetchMock.mock.calls.some((call) => String(call[0]).endsWith('/api/email-replies/70/resolve'))).toBe(true))
    await waitFor(() => expect(screen.queryByText(/possibly sent/i)).not.toBeInTheDocument())
  })

  it('keeps replying unavailable to read-only viewers', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({ ok: true, json: async () => ({ data: {
      messages: [{ id: 8, direction: 'inbound', fromEmail: 'customer@example.test', subject: 'Schedule', body: 'Does Tuesday work?', receivedAt: '2026-07-21T01:00:00Z' }], replies: []
    } }) })))

    render(<EmailThread messageId={8} canWrite={false} currentUserId={2} canManageReplies={false} />)
    expect(await screen.findByText(/does tuesday work/i)).toBeInTheDocument()
    expect(screen.queryByRole('textbox', { name: /reply to/i })).not.toBeInTheDocument()
  })
})

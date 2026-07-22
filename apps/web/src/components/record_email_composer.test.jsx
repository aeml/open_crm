import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { RecordEmailComposer } from './record_email_composer'
import { listEmailMessages, listRecordEmailDeliveries, resolveRecordEmailDelivery } from '../lib/email_messages'
import { listEmailTemplates, listEmailTemplateMergeFields, listEmailSnippets } from '../lib/email_templates'

vi.mock('../lib/email_messages', () => ({
  listEmailMessages: vi.fn(),
  listRecordEmailDeliveries: vi.fn(),
  resolveRecordEmailDelivery: vi.fn()
}))

vi.mock('../lib/email_templates', () => ({
  listEmailTemplates: vi.fn(),
  listEmailTemplateMergeFields: vi.fn(),
  listEmailSnippets: vi.fn()
}))

describe('RecordEmailComposer delivery recovery', () => {
  beforeEach(() => {
    listEmailMessages.mockResolvedValue([])
    listRecordEmailDeliveries.mockResolvedValue([])
    resolveRecordEmailDelivery.mockReset()
    listEmailTemplates.mockResolvedValue([])
    listEmailTemplateMergeFields.mockResolvedValue([])
    listEmailSnippets.mockResolvedValue([])
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('reuses one idempotency key after an uncertain provider response', async () => {
    const uncertainError = Object.assign(new Error('Check Sent mail.'), {
      payload: { error: { code: 'EMAIL_DELIVERY_UNCERTAIN' } }
    })
    const sendEmail = vi.fn()
      .mockRejectedValueOnce(uncertainError)
      .mockResolvedValueOnce({ id: 70, status: 'uncertain', to: 'ada@example.test' })

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={sendEmail}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    fireEvent.change(await screen.findByLabelText(/subject/i), { target: { value: 'Hello' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Body' } })
    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    expect(await screen.findByText(/check sent mail/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    await waitFor(() => expect(sendEmail).toHaveBeenCalledTimes(2))
    expect(sendEmail.mock.calls[0][2]).toMatch(/^record-email-/)
    expect(sendEmail.mock.calls[1][2]).toBe(sendEmail.mock.calls[0][2])
    expect(await screen.findByText(/delivery outcome is uncertain/i)).toBeInTheDocument()
  })

  it('reuses one idempotency key after an ambiguous browser transport failure', async () => {
    const sendEmail = vi.fn()
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({ id: 70, status: 'accepted', to: 'ada@example.test' })

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={sendEmail}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    fireEvent.change(await screen.findByLabelText(/subject/i), { target: { value: 'Hello' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Body' } })
    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    expect(await screen.findByText(/failed to fetch/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    await waitFor(() => expect(sendEmail).toHaveBeenCalledTimes(2))
    expect(sendEmail.mock.calls[1][2]).toBe(sendEmail.mock.calls[0][2])
    expect(await screen.findByText(/email sent to ada@example.test/i)).toBeInTheDocument()
  })

  it('isolates drafts, errors, and idempotency keys when the active record changes', async () => {
    const sendEmail = vi.fn()
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({ id: 71, status: 'accepted', to: 'grace@example.test' })
    const { rerender } = render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={sendEmail}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    fireEvent.change(await screen.findByLabelText(/subject/i), { target: { value: 'Ada draft' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Ada body' } })
    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    expect(await screen.findByText(/failed to fetch/i)).toBeInTheDocument()

    rerender(<RecordEmailComposer
      entityType="contact"
      entityId={9}
      canWrite
      recipientOptions={[{ id: 9, label: 'Grace <grace@example.test>' }]}
      sendEmail={sendEmail}
    />)
    expect(screen.queryByText(/failed to fetch/i)).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    expect(await screen.findByLabelText(/subject/i)).toHaveValue('')
    expect(screen.getByLabelText(/body/i)).toHaveValue('')
    fireEvent.change(screen.getByLabelText(/subject/i), { target: { value: 'Grace draft' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Grace body' } })
    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))

    await waitFor(() => expect(sendEmail).toHaveBeenCalledTimes(2))
    expect(sendEmail.mock.calls[0][0]).toBe(8)
    expect(sendEmail.mock.calls[1][0]).toBe(9)
    expect(sendEmail.mock.calls[1][2]).not.toBe(sendEmail.mock.calls[0][2])
  })

  it('surfaces an unresolved send and confirms it without another send call', async () => {
    listRecordEmailDeliveries
      .mockResolvedValueOnce([{
        id: 70,
        entityType: 'contact',
        entityId: 8,
        actorUserId: 1,
        to: 'ada@example.test',
        subject: 'Maybe sent',
        status: 'uncertain',
        lastError: 'Check the Sent folder.',
        ownedByCurrentUser: true,
        canRetry: true,
        canResolve: true
      }])
      .mockResolvedValue([])
    resolveRecordEmailDelivery.mockResolvedValue({ id: 70, status: 'accepted', to: 'ada@example.test' })
    const sendEmail = vi.fn()
    vi.spyOn(window, 'confirm').mockReturnValue(true)

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={sendEmail}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    expect(await screen.findByText('Maybe sent')).toBeInTheDocument()
    expect(screen.getByText(/resolve your in-progress or uncertain email/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /confirm sent/i }))

    await waitFor(() => expect(resolveRecordEmailDelivery).toHaveBeenCalledWith(70, 'confirmed_sent'))
    expect(sendEmail).not.toHaveBeenCalled()
    expect(await screen.findByText(/confirmed sent/i)).toBeInTheDocument()
  })

  it('surfaces unresolved recovery even when optional template tools fail', async () => {
    listEmailTemplates.mockRejectedValue(new Error('Template catalog unavailable'))
    listRecordEmailDeliveries.mockResolvedValue([{
      id: 72,
      entityType: 'contact',
      entityId: 8,
      actorUserId: 1,
      to: 'ada@example.test',
      subject: 'Needs recovery',
      status: 'uncertain',
      lastError: 'Check the Sent folder.',
      ownedByCurrentUser: true,
      canRetry: true,
      canResolve: true
    }])

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={vi.fn()}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    expect(await screen.findByText('Needs recovery')).toBeInTheDocument()
    expect(screen.getByText(/template catalog unavailable/i)).toBeInTheDocument()
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { RecordEmailComposer } from './record_email_composer'
import { listEmailMessages, listRecordEmailDeliveries, previewRecordEmail, resolveRecordEmailDelivery, sendRecordEmail, sendRecordEmailTest } from '../lib/email_messages'
import { listEmailTemplates, listEmailTemplateMergeFields, listEmailSnippets } from '../lib/email_templates'

vi.mock('../lib/email_messages', () => ({
  listEmailMessages: vi.fn(),
  listRecordEmailDeliveries: vi.fn(),
  previewRecordEmail: vi.fn(),
  sendRecordEmail: vi.fn(),
  sendRecordEmailTest: vi.fn(),
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
    previewRecordEmail.mockReset()
    sendRecordEmail.mockReset()
    sendRecordEmailTest.mockReset()
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
    sendRecordEmail
      .mockRejectedValueOnce(uncertainError)
      .mockResolvedValueOnce({ id: 70, status: 'uncertain', to: 'ada@example.test' })

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    fireEvent.change(await screen.findByLabelText(/subject/i), { target: { value: 'Hello' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Body' } })
    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    expect(await screen.findByText(/check sent mail/i)).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    await waitFor(() => expect(sendRecordEmail).toHaveBeenCalledTimes(2))
    expect(sendRecordEmail.mock.calls[0].slice(0, 2)).toEqual(['contact', 8])
    expect(sendRecordEmail.mock.calls[0][3]).toMatch(/^record-email-/)
    expect(sendRecordEmail.mock.calls[1][3]).toBe(sendRecordEmail.mock.calls[0][3])
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

  it('previews current merge values before sending a test only to the signed-in user', async () => {
    listEmailTemplates.mockResolvedValue([{
      id: 11,
      name: 'Regional hello',
      subject: 'Hello {{first_name}}',
      body: 'Your region is {{contact.custom.region}}.'
    }])
    previewRecordEmail.mockResolvedValue({
      to: 'ada@example.test',
      subject: 'Hello Ada',
      body: 'Your region is West.',
      unresolvedMergeFields: []
    })
    sendRecordEmailTest.mockResolvedValue({ status: 'accepted', to: 'owner@example.test' })

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={vi.fn()}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    fireEvent.change(await screen.findByLabelText(/template/i), { target: { value: '11' } })
    fireEvent.click(screen.getByRole('button', { name: /preview merged email/i }))

    const preview = await screen.findByRole('region', { name: /merged email preview/i })
    expect(preview).toHaveTextContent('Customer recipient: ada@example.test')
    expect(preview).toHaveTextContent('Subject: Hello Ada')
    expect(preview).toHaveTextContent('Your region is West.')
    expect(preview).toHaveTextContent(/all merge fields resolved/i)
    expect(previewRecordEmail).toHaveBeenCalledWith('contact', 8, {
      subject: 'Hello {{first_name}}',
      body: 'Your region is {{contact.custom.region}}.',
      trackEngagement: false
    })

    fireEvent.click(screen.getByRole('button', { name: /send test to me/i }))
    await waitFor(() => expect(sendRecordEmailTest).toHaveBeenCalledTimes(1))
    expect(sendRecordEmailTest.mock.calls[0].slice(0, 3)).toEqual([
      'contact',
      8,
      { subject: 'Hello {{first_name}}', body: 'Your region is {{contact.custom.region}}.', trackEngagement: false }
    ])
    expect(sendRecordEmailTest.mock.calls[0][3]).toMatch(/^record-email-/)
    expect(await screen.findByText(/test email sent only to owner@example.test/i)).toBeInTheDocument()
    expect(screen.getByText(/crm recipient was not emailed/i)).toBeInTheDocument()
  })

  it('blocks test delivery when the server preview reports unknown merge fields', async () => {
    previewRecordEmail.mockResolvedValue({
      to: 'ada@example.test',
      subject: 'Hello Ada',
      body: 'Unknown {{missing_field}}',
      unresolvedMergeFields: ['{{missing_field}}']
    })

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={vi.fn()}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    fireEvent.change(await screen.findByLabelText(/subject/i), { target: { value: 'Hello {{first_name}}' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Unknown {{missing_field}}' } })
    fireEvent.click(screen.getByRole('button', { name: /preview merged email/i }))

    expect(await screen.findByText(/unknown merge fields: \{\{missing_field\}\}/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /send test to me/i })).toBeDisabled()
    expect(sendRecordEmailTest).not.toHaveBeenCalled()
  })

  it('reuses the test idempotency key after an ambiguous browser failure', async () => {
    previewRecordEmail.mockResolvedValue({
      to: 'ada@example.test', subject: 'Hello Ada', body: 'Body', unresolvedMergeFields: []
    })
    sendRecordEmailTest
      .mockRejectedValueOnce(new TypeError('Failed to fetch'))
      .mockResolvedValueOnce({ status: 'accepted', to: 'owner@example.test' })

    render(<RecordEmailComposer
      entityType="contact"
      entityId={8}
      canWrite
      recipientOptions={[{ id: 8, label: 'Ada <ada@example.test>' }]}
      sendEmail={vi.fn()}
    />)

    fireEvent.click(screen.getByRole('button', { name: /^send email$/i }))
    fireEvent.change(await screen.findByLabelText(/subject/i), { target: { value: 'Hello Ada' } })
    fireEvent.change(screen.getByLabelText(/body/i), { target: { value: 'Body' } })
    fireEvent.click(screen.getByRole('button', { name: /preview merged email/i }))
    await screen.findByRole('region', { name: /merged email preview/i })

    fireEvent.click(screen.getByRole('button', { name: /send test to me/i }))
    expect(await screen.findByText(/failed to fetch/i)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /send test to me/i }))

    await waitFor(() => expect(sendRecordEmailTest).toHaveBeenCalledTimes(2))
    expect(sendRecordEmailTest.mock.calls[1][3]).toBe(sendRecordEmailTest.mock.calls[0][3])
    expect(await screen.findByText(/test email sent only to owner@example.test/i)).toBeInTheDocument()
  })
})

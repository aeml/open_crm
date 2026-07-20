import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { sendContactEmail } from '../lib/contacts'
import { listEmailMessages } from '../lib/email_messages'
import {
  cancelEmailSequenceEnrollment,
  createEmailSequenceEnrollment,
  listEmailSequenceEnrollments
} from '../lib/email_sequence_enrollments'
import { listEmailSequences } from '../lib/email_sequences'
import { listEmailTemplates } from '../lib/email_templates'
import { useContactOutreach } from './use_contact_outreach'

vi.mock('../lib/contacts', () => ({ sendContactEmail: vi.fn() }))
vi.mock('../lib/email_messages', () => ({ listEmailMessages: vi.fn() }))
vi.mock('../lib/email_sequence_enrollments', () => ({
  cancelEmailSequenceEnrollment: vi.fn(),
  createEmailSequenceEnrollment: vi.fn(),
  listEmailSequenceEnrollments: vi.fn()
}))
vi.mock('../lib/email_sequences', () => ({ listEmailSequences: vi.fn() }))
vi.mock('../lib/email_templates', () => ({ listEmailTemplates: vi.fn() }))

function deferred() {
  let resolve
  const promise = new Promise((next) => {
    resolve = next
  })
  return { promise, resolve }
}

beforeEach(() => {
  vi.clearAllMocks()
  sendContactEmail.mockResolvedValue({ sent: true })
  cancelEmailSequenceEnrollment.mockResolvedValue(undefined)
  createEmailSequenceEnrollment.mockResolvedValue({ id: 1, sequenceName: 'Nurture' })
  listEmailTemplates.mockResolvedValue([])
  listEmailSequences.mockResolvedValue([])
})

describe('useContactOutreach', () => {
  it('discards late email history when the active contact changes', async () => {
    const history = deferred()
    listEmailMessages.mockReturnValue(history.promise)
    const onError = vi.fn()
    const { result, rerender } = renderHook(
      ({ contactId }) => useContactOutreach({ selectedContactId: contactId, onError }),
      { initialProps: { contactId: 7 } }
    )

    let togglePromise
    act(() => {
      togglePromise = result.current.handleToggleEmail()
    })
    await waitFor(() => {
      expect(listEmailMessages).toHaveBeenCalledWith({ entityType: 'contact', entityId: 7 })
    })

    rerender({ contactId: 8 })
    expect(result.current.emailOpen).toBe(false)
    expect(result.current.emailHistory).toEqual([])
    rerender({ contactId: 7 })

    await act(async () => {
      history.resolve([{ id: 42, subject: 'Previous contact history' }])
      await togglePromise
    })

    expect(result.current.emailOpen).toBe(false)
    expect(result.current.emailHistory).toEqual([])
    expect(onError).not.toHaveBeenCalled()
  })

  it('discards late sequence enrollments when the active contact changes', async () => {
    const enrollments = deferred()
    listEmailSequences.mockResolvedValue([{ id: 4, name: 'Trial nurture' }])
    listEmailSequenceEnrollments.mockReturnValue(enrollments.promise)
    const onError = vi.fn()
    const { result, rerender } = renderHook(
      ({ contactId }) => useContactOutreach({ selectedContactId: contactId, onError }),
      { initialProps: { contactId: 7 } }
    )

    let togglePromise
    act(() => {
      togglePromise = result.current.handleToggleSequences()
    })
    await waitFor(() => {
      expect(listEmailSequenceEnrollments).toHaveBeenCalledWith({ contactId: 7 })
    })

    rerender({ contactId: 8 })
    expect(result.current.sequencesOpen).toBe(false)
    expect(result.current.sequenceEnrollments).toEqual([])

    await act(async () => {
      enrollments.resolve([{ id: 9, contactId: 7, sequenceName: 'Trial nurture' }])
      await togglePromise
    })

    expect(result.current.sequencesOpen).toBe(false)
    expect(result.current.sequenceEnrollments).toEqual([])
    expect(result.current.sequenceForm).toEqual({ sequenceId: '' })
    expect(onError).not.toHaveBeenCalled()
  })

  it('offers only active sequences approved for their current revision', async () => {
    listEmailSequences.mockResolvedValue([
      { id: 1, name: 'Draft', status: 'draft', revision: 1 },
      { id: 2, name: 'Stale approval', status: 'active', revision: 2, approvedRevision: 1, approvedAt: '2026-07-01T00:00:00Z' },
      { id: 3, name: 'Approved', status: 'active', revision: 2, approvedRevision: 2, approvedAt: '2026-07-02T00:00:00Z' }
    ])
    listEmailSequenceEnrollments.mockResolvedValue([])
    const { result } = renderHook(() => useContactOutreach({ selectedContactId: 7, onError: vi.fn() }))

    await act(async () => {
      await result.current.handleToggleSequences()
    })

    expect(result.current.sequenceOptions.map((sequence) => sequence.id)).toEqual([3])
    expect(result.current.sequenceForm).toEqual({ sequenceId: '3' })
  })
})

import { useLayoutEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import {
  cancelEmailSequenceEnrollment,
  createEmailSequenceEnrollment,
  listEmailSequenceEnrollments
} from '../lib/email_sequence_enrollments'
import { listEmailSequences } from '../lib/email_sequences'

const emptySequenceForm = { sequenceId: '' }

// Sequence outreach is record-scoped. Keep workspace-wide definition options
// cached, but synchronously clear record-specific state when selection changes
// and ignore responses belonging to a contact that is no longer active.
export function useContactOutreach({ selectedContactId, onError }) {
  const activeSelectionRef = useRef({ contactId: selectedContactId })
  if (activeSelectionRef.current.contactId !== selectedContactId) {
    activeSelectionRef.current = { contactId: selectedContactId }
  }
  const [sequencesOpen, setSequencesOpen] = useState(false)
  const [sequenceOptions, setSequenceOptions] = useState([])
  const [sequenceEnrollments, setSequenceEnrollments] = useState([])
  const [sequenceForm, setSequenceForm] = useState(emptySequenceForm)
  const [sequenceStatus, setSequenceStatus] = useState('')
  const [isEnrollingSequence, setIsEnrollingSequence] = useState(false)

  useLayoutEffect(() => {
    setSequencesOpen(false)
    setSequenceEnrollments([])
    setSequenceForm(emptySequenceForm)
    setSequenceStatus('')
    setIsEnrollingSequence(false)
  }, [selectedContactId])

  async function handleToggleSequences() {
    const next = !sequencesOpen
    const selection = activeSelectionRef.current
    const contactId = selection.contactId
    setSequencesOpen(next)
    setSequenceStatus('')
    if (!next || !contactId) {
      return
    }
    try {
      const [sequences, enrollments] = await Promise.all([
        listEmailSequences(),
        listEmailSequenceEnrollments({ contactId })
      ])
      if (activeSelectionRef.current !== selection) {
        return
      }
      const approvedSequences = sequences.filter((sequence) => sequence.status === 'active' && sequence.approvedAt && sequence.approvedRevision === sequence.revision)
      setSequenceOptions(approvedSequences)
      setSequenceEnrollments(enrollments)
      setSequenceForm({ sequenceId: approvedSequences[0]?.id ? String(approvedSequences[0].id) : '' })
    } catch (loadError) {
      if (!isAbortError(loadError) && activeSelectionRef.current === selection) {
        onError(loadError.message || 'Unable to load email sequences.')
      }
    }
  }

  async function handleEnrollSequence(event) {
    event.preventDefault()
    const selection = activeSelectionRef.current
    const contactId = selection.contactId
    if (!contactId || !sequenceForm.sequenceId) {
      return
    }
    setIsEnrollingSequence(true)
    setSequenceStatus('')
    try {
      const enrollment = await createEmailSequenceEnrollment({
        contactId,
        sequenceId: Number.parseInt(sequenceForm.sequenceId, 10)
      })
      if (activeSelectionRef.current !== selection) {
        return
      }
      setSequenceEnrollments((current) => [enrollment, ...current.filter((entry) => entry.id !== enrollment.id)])
      setSequenceStatus(`Enrolled in ${enrollment.sequenceName || 'sequence'}.`)
      onError('')
    } catch (enrollError) {
      if (activeSelectionRef.current === selection) {
        onError(enrollError.message || 'Unable to enroll contact in sequence.')
      }
    } finally {
      if (activeSelectionRef.current === selection) {
        setIsEnrollingSequence(false)
      }
    }
  }

  async function handleCancelSequenceEnrollment(enrollmentId) {
    const selection = activeSelectionRef.current
    try {
      await cancelEmailSequenceEnrollment(enrollmentId)
      if (activeSelectionRef.current !== selection) {
        return
      }
      setSequenceEnrollments((current) => current.filter((entry) => entry.id !== enrollmentId))
      setSequenceStatus('Sequence enrollment cancelled.')
      onError('')
    } catch (cancelError) {
      if (activeSelectionRef.current === selection) {
        onError(cancelError.message || 'Unable to cancel sequence enrollment.')
      }
    }
  }

  return {
    handleCancelSequenceEnrollment,
    handleEnrollSequence,
    handleToggleSequences,
    isEnrollingSequence,
    sequenceEnrollments,
    sequenceForm,
    sequenceOptions,
    sequencesOpen,
    sequenceStatus,
    setSequenceForm
  }
}

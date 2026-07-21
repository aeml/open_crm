import { useLayoutEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { sendContactEmail } from '../lib/contacts'
import { listEmailMessages } from '../lib/email_messages'
import {
  cancelEmailSequenceEnrollment,
  createEmailSequenceEnrollment,
  listEmailSequenceEnrollments
} from '../lib/email_sequence_enrollments'
import { listEmailSequences } from '../lib/email_sequences'
import { listEmailTemplates } from '../lib/email_templates'

const emptyEmailForm = { subject: '', body: '', trackEngagement: false }
const emptySequenceForm = { sequenceId: '' }

// Contact outreach is record-scoped. Keep workspace-wide template/sequence
// options cached, but synchronously clear record-specific state when selection
// changes and ignore responses belonging to a contact that is no longer active.
export function useContactOutreach({ selectedContactId, onError }) {
  const activeSelectionRef = useRef({ contactId: selectedContactId })
  if (activeSelectionRef.current.contactId !== selectedContactId) {
    activeSelectionRef.current = { contactId: selectedContactId }
  }
  const [emailOpen, setEmailOpen] = useState(false)
  const [emailTemplates, setEmailTemplates] = useState([])
  const [emailForm, setEmailForm] = useState(emptyEmailForm)
  const [emailStatus, setEmailStatus] = useState('')
  const [isSendingEmail, setIsSendingEmail] = useState(false)
  const [emailHistory, setEmailHistory] = useState([])
  const [sequencesOpen, setSequencesOpen] = useState(false)
  const [sequenceOptions, setSequenceOptions] = useState([])
  const [sequenceEnrollments, setSequenceEnrollments] = useState([])
  const [sequenceForm, setSequenceForm] = useState(emptySequenceForm)
  const [sequenceStatus, setSequenceStatus] = useState('')
  const [isEnrollingSequence, setIsEnrollingSequence] = useState(false)

  useLayoutEffect(() => {
    setEmailOpen(false)
    setEmailForm(emptyEmailForm)
    setEmailStatus('')
    setIsSendingEmail(false)
    setEmailHistory([])
    setSequencesOpen(false)
    setSequenceEnrollments([])
    setSequenceForm(emptySequenceForm)
    setSequenceStatus('')
    setIsEnrollingSequence(false)
  }, [selectedContactId])

  async function handleToggleEmail() {
    const next = !emailOpen
    const selection = activeSelectionRef.current
    const contactId = selection.contactId
    setEmailOpen(next)
    setEmailStatus('')
    if (!next) {
      return
    }
    if (emailTemplates.length === 0) {
      try {
        const templates = await listEmailTemplates()
        setEmailTemplates(templates)
      } catch (templatesError) {
        if (!isAbortError(templatesError)) {
          setEmailTemplates([])
        }
      }
    }
    if (!contactId) {
      return
    }
    if (activeSelectionRef.current !== selection) {
      return
    }
    try {
      const history = await listEmailMessages({ entityType: 'contact', entityId: contactId })
      if (activeSelectionRef.current === selection) {
        setEmailHistory(history)
      }
    } catch (historyError) {
      if (!isAbortError(historyError) && activeSelectionRef.current === selection) {
        setEmailHistory([])
      }
    }
  }

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

  function applyEmailTemplate(templateId) {
    const template = emailTemplates.find((item) => String(item.id) === String(templateId))
    if (template) {
      setEmailForm((current) => ({ ...current, subject: template.subject, body: template.body }))
    }
  }

  async function handleSendEmail(event) {
    event.preventDefault()
    const selection = activeSelectionRef.current
    const contactId = selection.contactId
    if (!contactId) {
      return
    }
    setIsSendingEmail(true)
    setEmailStatus('')
    try {
      const result = await sendContactEmail(contactId, emailForm)
      if (activeSelectionRef.current !== selection) {
        return
      }
      setEmailStatus(`Email sent to ${result?.to || 'contact'}.`)
      setEmailForm(emptyEmailForm)
      onError('')
      try {
        const history = await listEmailMessages({ entityType: 'contact', entityId: contactId })
        if (activeSelectionRef.current === selection) {
          setEmailHistory(history)
        }
      } catch (historyError) {
        if (!isAbortError(historyError)) {
          // History refresh is best-effort; the send result remains authoritative.
        }
      }
    } catch (sendError) {
      if (activeSelectionRef.current === selection) {
        onError(sendError.message || 'Unable to send email.')
      }
    } finally {
      if (activeSelectionRef.current === selection) {
        setIsSendingEmail(false)
      }
    }
  }

  return {
    applyEmailTemplate,
    emailForm,
    emailHistory,
    emailOpen,
    emailStatus,
    emailTemplates,
    handleCancelSequenceEnrollment,
    handleEnrollSequence,
    handleSendEmail,
    handleToggleEmail,
    handleToggleSequences,
    isEnrollingSequence,
    isSendingEmail,
    sequenceEnrollments,
    sequenceForm,
    sequenceOptions,
    sequencesOpen,
    sequenceStatus,
    setEmailForm,
    setSequenceForm
  }
}

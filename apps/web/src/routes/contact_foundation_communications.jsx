import { useEffect, useState } from 'react'
import { isAbortError } from '../lib/api'
import { cancelCalendarEvent, listCalendarEvents, scheduleCalendarEvent } from '../lib/calendar'
import { completeCall, listCalls, logCall, startCall, updateCallRecording } from '../lib/calls'
import { listSMSMessages, logInboundSMS, optOutSMS, sendContactSMS } from '../lib/sms'
import { ContactCallsCard } from './contact_calls'
import { ContactMeetingsCard, ContactSMSCard, smsTemplates } from './contact_communications'
import {
  defaultMeetingTimezone,
  emptyCallForm,
  emptyInboundSMSForm,
  emptyManualCallForm,
  emptyMeetingForm,
  emptyRecordingForm,
  emptySMSForm,
  localDateTimeToISOString,
  recordingFormValues
} from './contact_view'

export function ContactFoundationCommunications({ canWrite, contact, contactId, onError, onSnapshotChange }) {
  const [callsOpen, setCallsOpen] = useState(false)
  const [callLogs, setCallLogs] = useState([])
  const [activeCall, setActiveCall] = useState(null)
  const [callDialURL, setCallDialURL] = useState('')
  const [callForm, setCallForm] = useState(emptyCallForm)
  const [inboundCallOpen, setInboundCallOpen] = useState(false)
  const [manualCallForm, setManualCallForm] = useState(emptyManualCallForm)
  const [recordingCallId, setRecordingCallId] = useState(null)
  const [recordingForm, setRecordingForm] = useState(emptyRecordingForm)
  const [callStatus, setCallStatus] = useState('')
  const [isStartingCall, setIsStartingCall] = useState(false)
  const [isCompletingCall, setIsCompletingCall] = useState(false)
  const [isLoggingCall, setIsLoggingCall] = useState(false)
  const [isUpdatingRecording, setIsUpdatingRecording] = useState(false)
  const [smsOpen, setSmsOpen] = useState(false)
  const [smsMessages, setSmsMessages] = useState([])
  const [smsForm, setSmsForm] = useState(emptySMSForm)
  const [inboundSMSOpen, setInboundSMSOpen] = useState(false)
  const [inboundSMSForm, setInboundSMSForm] = useState(emptyInboundSMSForm)
  const [smsStatus, setSmsStatus] = useState('')
  const [isSendingSMS, setIsSendingSMS] = useState(false)
  const [isLoggingInboundSMS, setIsLoggingInboundSMS] = useState(false)
  const [isOptingOutSMS, setIsOptingOutSMS] = useState(false)
  const [meetingsOpen, setMeetingsOpen] = useState(false)
  const [meetingEvents, setMeetingEvents] = useState([])
  const [meetingForm, setMeetingForm] = useState(emptyMeetingForm)
  const [meetingStatus, setMeetingStatus] = useState('')
  const [isSchedulingMeeting, setIsSchedulingMeeting] = useState(false)
  const [cancellingMeetingId, setCancellingMeetingId] = useState(null)

  useEffect(() => {
    onSnapshotChange(JSON.stringify({ callLogs, meetingEvents, smsMessages }))
  }, [callLogs, meetingEvents, onSnapshotChange, smsMessages])

  async function handleToggleCalls() {
    const next = !callsOpen
    setCallsOpen(next)
    setCallStatus('')
    if (!next || !contactId) {
      return
    }
    try {
      setCallLogs(await listCalls({ entityType: 'contact', entityId: contactId }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        onError(loadError.message || 'Unable to load call history.')
      }
    }
  }

  async function handleToggleSMS() {
    const next = !smsOpen
    setSmsOpen(next)
    setSmsStatus('')
    if (!next || !contactId) {
      return
    }
    try {
      setSmsMessages(await listSMSMessages({ entityType: 'contact', entityId: contactId }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        onError(loadError.message || 'Unable to load SMS history.')
      }
    }
  }

  function applySMSTemplate(templateName) {
    const template = smsTemplates.find((entry) => entry.name === templateName)
    setSmsForm({ templateName, body: template?.body || '' })
  }

  async function handleSendSMS(event) {
    event.preventDefault()
    if (!contactId || !contact?.phone || !smsForm.body.trim()) {
      return
    }
    setIsSendingSMS(true)
    setSmsStatus('')
    try {
      const message = await sendContactSMS(contactId, {
        body: smsForm.body.trim(),
        templateName: smsForm.templateName
      })
      setSmsMessages((current) => [message, ...current.filter((entry) => entry.id !== message.id)])
      setSmsOpen(true)
      setSmsForm(emptySMSForm)
      setSmsStatus(message.status === 'suppressed' ? 'SMS suppressed because this phone number is opted out.' : (message.status === 'failed' ? 'SMS failed.' : 'SMS sent.'))
      onError('')
    } catch (sendError) {
      onError(sendError.message || 'Unable to send SMS.')
    } finally {
      setIsSendingSMS(false)
    }
  }

  function handleToggleInboundSMSLog() {
    setInboundSMSOpen((current) => !current)
    setSmsStatus('')
  }

  async function handleLogInboundSMS(event) {
    event.preventDefault()
    if (!contactId || !contact?.phone || !inboundSMSForm.body.trim()) {
      return
    }
    setIsLoggingInboundSMS(true)
    setSmsStatus('')
    try {
      const message = await logInboundSMS({
        entityType: 'contact',
        entityId: contactId,
        phoneNumber: contact.phone,
        body: inboundSMSForm.body.trim()
      })
      setSmsMessages((current) => [message, ...current.filter((entry) => entry.id !== message.id)])
      setSmsOpen(true)
      setInboundSMSOpen(false)
      setInboundSMSForm(emptyInboundSMSForm)
      setSmsStatus('Inbound SMS logged. STOP-style replies opt the number out automatically.')
      onError('')
    } catch (logError) {
      onError(logError.message || 'Unable to log inbound SMS.')
    } finally {
      setIsLoggingInboundSMS(false)
    }
  }

  async function handleSMSOptOut() {
    if (!contactId || !contact?.phone) {
      return
    }
    setIsOptingOutSMS(true)
    setSmsStatus('')
    try {
      await optOutSMS({
        phoneNumber: contact.phone,
        reason: 'manual',
        source: 'contact_detail',
        entityType: 'contact',
        entityId: contactId
      })
      setSmsStatus('SMS opt-out recorded.')
      onError('')
    } catch (optOutError) {
      onError(optOutError.message || 'Unable to opt out phone number.')
    } finally {
      setIsOptingOutSMS(false)
    }
  }

  async function handleToggleMeetings() {
    const next = !meetingsOpen
    setMeetingsOpen(next)
    setMeetingStatus('')
    if (!next || !contactId) {
      return
    }
    try {
      setMeetingEvents(await listCalendarEvents({ entityType: 'contact', entityId: contactId }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        onError(loadError.message || 'Unable to load meetings.')
      }
    }
  }

  async function handleScheduleMeeting(event) {
    event.preventDefault()
    if (!contactId || !meetingForm.title.trim()) {
      return
    }
    const startAt = localDateTimeToISOString(meetingForm.startAt)
    const endAt = localDateTimeToISOString(meetingForm.endAt)
    if (!startAt || !endAt) {
      onError('Meeting start and end are required.')
      return
    }
    setIsSchedulingMeeting(true)
    setMeetingStatus('')
    try {
      const meeting = await scheduleCalendarEvent({
        entityType: 'contact',
        entityId: contactId,
        title: meetingForm.title.trim(),
        description: meetingForm.description.trim(),
        location: meetingForm.location.trim(),
        startAt,
        endAt,
        timezone: meetingForm.timezone.trim() || defaultMeetingTimezone(),
        visibility: meetingForm.visibility
      })
      setMeetingEvents((current) => [meeting, ...current.filter((entry) => entry.id !== meeting.id)])
      setMeetingsOpen(true)
      setMeetingForm(emptyMeetingForm())
      setMeetingStatus('Meeting scheduled.')
      onError('')
    } catch (scheduleError) {
      onError(scheduleError.message || 'Unable to schedule meeting.')
    } finally {
      setIsSchedulingMeeting(false)
    }
  }

  async function handleCancelMeeting(eventId) {
    setCancellingMeetingId(eventId)
    setMeetingStatus('')
    try {
      const meeting = await cancelCalendarEvent(eventId)
      setMeetingEvents((current) => current.map((entry) => entry.id === meeting.id ? meeting : entry))
      setMeetingStatus('Meeting cancelled.')
      onError('')
    } catch (cancelError) {
      onError(cancelError.message || 'Unable to cancel meeting.')
    } finally {
      setCancellingMeetingId(null)
    }
  }

  async function handleStartCall() {
    if (!contactId || !contact?.phone) {
      return
    }
    setIsStartingCall(true)
    setCallStatus('')
    try {
      const result = await startCall({ entityType: 'contact', entityId: contactId, phoneNumber: contact.phone })
      if (result?.call) {
        setActiveCall(result.call)
        setCallLogs((current) => [result.call, ...current.filter((call) => call.id !== result.call.id)])
      }
      setCallDialURL(result?.dialUrl || '')
      setCallsOpen(true)
      setCallStatus('Call started. Log the outcome when you finish.')
      onError('')
    } catch (startError) {
      onError(startError.message || 'Unable to start call.')
    } finally {
      setIsStartingCall(false)
    }
  }

  async function handleCompleteCall(event) {
    event.preventDefault()
    if (!activeCall?.id) {
      return
    }
    setIsCompletingCall(true)
    setCallStatus('')
    try {
      const call = await completeCall(activeCall.id, {
        status: 'completed',
        disposition: callForm.disposition.trim(),
        notes: callForm.notes.trim()
      })
      setCallLogs((current) => [call, ...current.filter((entry) => entry.id !== call.id)])
      setActiveCall(null)
      setCallDialURL('')
      setCallForm(emptyCallForm)
      setCallStatus('Call outcome logged.')
      onError('')
    } catch (completeError) {
      onError(completeError.message || 'Unable to log call outcome.')
    } finally {
      setIsCompletingCall(false)
    }
  }

  function handleToggleInboundCallLog() {
    setInboundCallOpen((current) => {
      const next = !current
      if (next) {
        setManualCallForm((form) => ({ ...form, phoneNumber: form.phoneNumber || contact?.phone || '' }))
      }
      return next
    })
    setCallStatus('')
  }

  async function handleRecordInboundCall(event) {
    event.preventDefault()
    if (!contactId || !manualCallForm.phoneNumber.trim()) {
      return
    }
    setIsLoggingCall(true)
    setCallStatus('')
    try {
      const call = await logCall({
        entityType: 'contact',
        entityId: contactId,
        direction: 'inbound',
        phoneNumber: manualCallForm.phoneNumber.trim(),
        status: 'completed',
        disposition: manualCallForm.disposition.trim(),
        notes: manualCallForm.notes.trim()
      })
      setCallLogs((current) => [call, ...current.filter((entry) => entry.id !== call.id)])
      setCallsOpen(true)
      setInboundCallOpen(false)
      setManualCallForm(emptyManualCallForm)
      setCallStatus('Inbound call logged.')
      onError('')
    } catch (logError) {
      onError(logError.message || 'Unable to log inbound call.')
    } finally {
      setIsLoggingCall(false)
    }
  }

  function handleToggleRecordingControls(call) {
    setCallStatus('')
    if (recordingCallId === call.id) {
      setRecordingCallId(null)
      setRecordingForm(emptyRecordingForm)
      return
    }
    setRecordingCallId(call.id)
    setRecordingForm(recordingFormValues(call))
  }

  async function handleUpdateCallRecording(event) {
    event.preventDefault()
    if (!recordingCallId) {
      return
    }
    setIsUpdatingRecording(true)
    setCallStatus('')
    try {
      const call = await updateCallRecording(recordingCallId, {
        recordingUrl: recordingForm.recordingUrl.trim(),
        recordingConsent: recordingForm.recordingConsent,
        retentionDays: Number.parseInt(recordingForm.retentionDays, 10) || 365,
        deleteRecording: false
      })
      setCallLogs((current) => current.map((entry) => entry.id === call.id ? call : entry))
      setRecordingCallId(null)
      setRecordingForm(emptyRecordingForm)
      setCallStatus('Call recording controls updated.')
      onError('')
    } catch (recordingError) {
      onError(recordingError.message || 'Unable to update call recording.')
    } finally {
      setIsUpdatingRecording(false)
    }
  }

  async function handleDeleteCallRecording() {
    if (!recordingCallId) {
      return
    }
    setIsUpdatingRecording(true)
    setCallStatus('')
    try {
      const call = await updateCallRecording(recordingCallId, {
        recordingConsent: recordingForm.recordingConsent,
        deleteRecording: true
      })
      setCallLogs((current) => current.map((entry) => entry.id === call.id ? call : entry))
      setRecordingCallId(null)
      setRecordingForm(emptyRecordingForm)
      setCallStatus('Call recording deleted.')
      onError('')
    } catch (recordingError) {
      onError(recordingError.message || 'Unable to delete call recording.')
    } finally {
      setIsUpdatingRecording(false)
    }
  }

  return (
    <>
      <ContactCallsCard
        activeCall={activeCall}
        callForm={callForm}
        calls={callLogs}
        canWrite={canWrite}
        contact={contact}
        dialURL={callDialURL}
        inboundForm={manualCallForm}
        inboundOpen={inboundCallOpen}
        isCompleting={isCompletingCall}
        isLogging={isLoggingCall}
        isStarting={isStartingCall}
        isUpdatingRecording={isUpdatingRecording}
        onComplete={handleCompleteCall}
        onDeleteRecording={handleDeleteCallRecording}
        onRecordInbound={handleRecordInboundCall}
        onSetCallForm={setCallForm}
        onSetInboundForm={setManualCallForm}
        onSetRecordingForm={setRecordingForm}
        onStart={handleStartCall}
        onToggle={handleToggleCalls}
        onToggleInbound={handleToggleInboundCallLog}
        onToggleRecording={handleToggleRecordingControls}
        onUpdateRecording={handleUpdateCallRecording}
        open={callsOpen}
        recordingCallId={recordingCallId}
        recordingForm={recordingForm}
        status={callStatus}
      />
      <ContactSMSCard
        canWrite={canWrite}
        contact={contact}
        inboundForm={inboundSMSForm}
        inboundOpen={inboundSMSOpen}
        isLoggingInbound={isLoggingInboundSMS}
        isOptingOut={isOptingOutSMS}
        isSending={isSendingSMS}
        messages={smsMessages}
        onApplyTemplate={applySMSTemplate}
        onLogInbound={handleLogInboundSMS}
        onOptOut={handleSMSOptOut}
        onSend={handleSendSMS}
        onSetForm={setSmsForm}
        onSetInboundForm={setInboundSMSForm}
        onToggle={handleToggleSMS}
        onToggleInbound={handleToggleInboundSMSLog}
        open={smsOpen}
        form={smsForm}
        status={smsStatus}
      />
      <ContactMeetingsCard
        canWrite={canWrite}
        cancellingMeetingId={cancellingMeetingId}
        events={meetingEvents}
        form={meetingForm}
        isScheduling={isSchedulingMeeting}
        onCancel={handleCancelMeeting}
        onSchedule={handleScheduleMeeting}
        onSetForm={setMeetingForm}
        onToggle={handleToggleMeetings}
        open={meetingsOpen}
        status={meetingStatus}
      />
    </>
  )
}

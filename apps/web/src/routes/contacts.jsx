import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { bulkStatusOptions } from '../components/ui/bulk_actions'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { cancelCalendarEvent, listCalendarEvents, scheduleCalendarEvent } from '../lib/calendar'
import { completeCall, listCalls, logCall, startCall, updateCallRecording } from '../lib/calls'
import { archiveContact, contactsExportURL, createContact, getContact, listContacts, sendContactEmail, updateContact } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { createNote } from '../lib/notes'
import { listEmailSequences } from '../lib/email_sequences'
import { cancelEmailSequenceEnrollment, createEmailSequenceEnrollment, listEmailSequenceEnrollments } from '../lib/email_sequence_enrollments'
import { listEmailTemplates } from '../lib/email_templates'
import { listEmailMessages } from '../lib/email_messages'
import { evaluateContactLeadScore } from '../lib/lead_scoring'
import { listSMSMessages, logInboundSMS, optOutSMS, sendContactSMS } from '../lib/sms'
import { createTask, listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'
import { customFieldFilterFromParams, listCustomFields } from '../lib/custom_fields'
import { usePageTitle } from '../lib/use_page_title'
import { ContactCallsCard } from './contact_calls'
import { ContactEmailCard, ContactMeetingsCard, ContactSMSCard, ContactSequencesCard, smsTemplates } from './contact_communications'
import { ContactForm } from './contact_form'
import { ContactAttributionCard, ContactLeadScoreCard } from './contact_insights'
import { ClientAccountContext } from './client_account_context'
import { ContactListCard } from './contact_list'
import {
  contactFormValues,
  contactPayload,
  defaultMeetingTimezone,
  duplicateSearchTerm,
  emptyCallForm,
  emptyContactForm,
  emptyContactTaskForm,
  emptyInboundSMSForm,
  emptyManualCallForm,
  emptyMeetingForm,
  emptyRecordingForm,
  emptySMSForm,
  formatContactAddress,
  fullContactName,
  localDateTimeToISOString,
  recordingFormValues,
  relatedPipelineLabels
} from './contact_view'
import { RecordWorkCards } from './record_work'
import { TouchpointSummary } from './touchpoint_summary'

export function ContactsRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { contactId } = useParams()
  const { session, businessProfile } = useAuth()
  const routeContactId = Number.parseInt(contactId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const canWrite = ['owner', 'admin', 'member'].includes(session?.membership?.role)
  const pipelineLabels = relatedPipelineLabels(businessType)
  usePageTitle('Contacts')
  const initialSearch = searchParams.get('q') || ''
  const initialOwnerFilter = searchParams.get('owner') || 'all'
  const initialCustomFilter = customFieldFilterFromParams(searchParams)
  const [mode, setMode] = useState('list')
  const [contacts, setContacts] = useState([])
  const [selectedContactIds, setSelectedContactIds] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState(initialSearch)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [customFilter, setCustomFilter] = useState(initialCustomFilter)
  const [selectedContactId, setSelectedContactId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [userOptions, setUserOptions] = useState([])
  const [form, setForm] = useState(emptyContactForm)
  const [customDefinitions, setCustomDefinitions] = useState([])
  const [customDefinitionsLoaded, setCustomDefinitionsLoaded] = useState(false)
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyContactTaskForm)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [duplicateCandidate, setDuplicateCandidate] = useState(null)
  const [emailOpen, setEmailOpen] = useState(false)
  const [emailTemplates, setEmailTemplates] = useState([])
  const [emailForm, setEmailForm] = useState({ subject: '', body: '' })
  const [emailStatus, setEmailStatus] = useState('')
  const [isSendingEmail, setIsSendingEmail] = useState(false)
  const [emailHistory, setEmailHistory] = useState([])
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
  const [sequencesOpen, setSequencesOpen] = useState(false)
  const [sequenceOptions, setSequenceOptions] = useState([])
  const [sequenceEnrollments, setSequenceEnrollments] = useState([])
  const [sequenceForm, setSequenceForm] = useState({ sequenceId: '' })
  const [sequenceStatus, setSequenceStatus] = useState('')
  const [leadScoreStatus, setLeadScoreStatus] = useState('')
  const [isEnrollingSequence, setIsEnrollingSequence] = useState(false)
  const [isEvaluatingLeadScore, setIsEvaluatingLeadScore] = useState(false)
  const searchControllerRef = useRef(null)

  const selectedContact = detail?.contact || null
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedDeals = detail?.deals || []
  const hasFilter = search.trim() !== '' || ownerFilter !== 'all' || customFilter.fieldKey !== ''
  const selectedActivities = detail?.activities || []

  useLayoutEffect(() => {
    setCallsOpen(false)
    setCallLogs([])
    setActiveCall(null)
    setCallDialURL('')
    setCallForm(emptyCallForm)
    setInboundCallOpen(false)
    setManualCallForm(emptyManualCallForm)
    setRecordingCallId(null)
    setRecordingForm(emptyRecordingForm)
    setCallStatus('')
    setSmsOpen(false)
    setSmsMessages([])
    setSmsForm(emptySMSForm)
    setInboundSMSOpen(false)
    setInboundSMSForm(emptyInboundSMSForm)
    setSmsStatus('')
    setMeetingsOpen(false)
    setMeetingEvents([])
    setMeetingForm(emptyMeetingForm())
    setMeetingStatus('')
    setSequencesOpen(false)
    setSequenceEnrollments([])
    setSequenceStatus('')
    setSequenceForm({ sequenceId: '' })
    setLeadScoreStatus('')
  }, [selectedContactId])

  function buildContactsPath(nextSearch = search, nextOwner = ownerFilter, nextCustomFilter = customFilter) {
    const params = new URLSearchParams()
    if (nextSearch) {
      params.set('q', nextSearch)
    }
    if (nextOwner !== 'all') {
      params.set('owner', nextOwner)
    }
    if (nextCustomFilter.fieldKey) {
      params.set('customField', nextCustomFilter.fieldKey)
      params.set('customOperator', nextCustomFilter.operator)
      params.set('customValue', nextCustomFilter.value)
    }
    const suffix = params.toString() ? `?${params.toString()}` : ''
    return `/contacts${suffix}`
  }

  async function loadContacts(nextSearch = '', nextOwner = 'all', { signal, nextCustomFilter = customFilter } = {}) {
    const isUnassigned = nextOwner === 'unassigned'
    const ownerUserId = isUnassigned || nextOwner === 'all' ? 0 : Number.parseInt(nextOwner, 10) || 0
    const data = await listContacts({ search: nextSearch, unassigned: isUnassigned, ownerUserId, customField: nextCustomFilter }, { signal })

    if (Array.isArray(data?.contacts)) {
      setContacts(data.contacts)
      setSelectedContactIds([])
      setMeta(data.meta || { page: 1, pageSize: 20, total: data.contacts.length })
      return
    }

    if (data?.contact) {
      const entry = data.contact
      setContacts([entry])
      setMeta({ page: 1, pageSize: 20, total: 1 })
      setDetailCache((current) => ({ ...current, [entry.id]: data }))
      return
    }

    setContacts([])
    setMeta({ page: 1, pageSize: 20, total: 0 })
  }

  async function loadUserOptions({ signal } = {}) {
    const nextUsers = await listOrganizationUsers({ signal })
    setUserOptions(nextUsers)
    setTaskForm((current) => {
      if (current.assignedToUserId || nextUsers.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(nextUsers[0].id) }
    })
  }

  async function loadCustomDefinitions({ signal } = {}) {
    const definitions = await listCustomFields('contact', { signal })
    setCustomDefinitions(definitions)
    setCustomDefinitionsLoaded(true)
    return definitions
  }

  useEffect(() => {
    const controller = new AbortController()

    async function run() {
      setIsListLoading(true)
      try {
        await Promise.all([loadContacts(initialSearch, initialOwnerFilter, { signal: controller.signal }), loadUserOptions({ signal: controller.signal }), loadCustomDefinitions({ signal: controller.signal })])
        setError('')
        setDuplicateSearch('')
        setDuplicateCandidate(null)
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load contacts.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsListLoading(false)
        }
      }
    }

    run()
    return () => {
      controller.abort()
    }
  }, [])

  async function handleSearchChange(event) {
    const value = event.target.value
    setSearch(value)
    navigate(buildContactsPath(value, ownerFilter), { replace: true })
    await reloadContacts(value, ownerFilter)
  }

  async function handleApplySavedView(filters) {
    const nextSearch = filters.q || ''
    const nextOwner = filters.owner || 'all'
    const nextCustomFilter = { fieldKey: filters.customField || '', operator: filters.customOperator || '', value: filters.customValue || '' }
    setSearch(nextSearch)
    setOwnerFilter(nextOwner)
    setCustomFilter(nextCustomFilter)
    setMode('list')
    setDetail(null)
    setSelectedContactId(null)
    navigate(buildContactsPath(nextSearch, nextOwner, nextCustomFilter), { replace: true })
    await reloadContacts(nextSearch, nextOwner, nextCustomFilter)
  }

  async function reloadContacts(nextSearch = search, nextOwner = ownerFilter, nextCustomFilter = customFilter) {
    searchControllerRef.current?.abort()
    const controller = new AbortController()
    searchControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadContacts(nextSearch, nextOwner, { signal: controller.signal, nextCustomFilter })
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load contacts.')
      }
    } finally {
      if (searchControllerRef.current === controller) {
        searchControllerRef.current = null
      }
      if (!controller.signal.aborted) {
        setIsListLoading(false)
      }
    }
  }

  async function applyOwnerFilter(value) {
    setOwnerFilter(value)
    navigate(buildContactsPath(search, value), { replace: true })
    await reloadContacts(search, value)
  }

  async function handleOwnerFilterChange(event) {
    await applyOwnerFilter(event.target.value)
  }

  async function applyCustomFilter(nextCustomFilter) {
    setCustomFilter(nextCustomFilter)
    navigate(buildContactsPath(search, ownerFilter, nextCustomFilter), { replace: true })
    await reloadContacts(search, ownerFilter, nextCustomFilter)
  }

  async function handleDuplicateSearch() {
    if (!duplicateSearch) {
      return
    }

    setSearch(duplicateSearch)
    try {
      await loadContacts(duplicateSearch)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load contacts.')
    }
  }

  function handleOpenDuplicate() {
    if (!duplicateCandidate?.id) {
      return
    }
    navigate(`/contacts/${duplicateCandidate.id}`)
  }

  function handleOpenDeal(dealID) {
    navigate(`/deals/${dealID}`)
  }

  function handleCreateRelatedDeal() {
    if (!selectedContactId) {
      return
    }
    const params = new URLSearchParams({ primaryContactId: String(selectedContactId) })
    navigate(`/deals?${params.toString()}`)
  }

  function handleOpenContactTasks() {
    if (!selectedContactId) {
      return
    }
    navigate(`/tasks?entityType=contact&entityId=${selectedContactId}`)
  }

  async function handleOpenContact(contact) {
    const contactID = contact.id
    const cached = detailCache[contactID]
    if (cached) {
      setSelectedContactId(contactID)
      setDetail(cached)
      setForm(contactFormValues(cached.contact, customDefinitions))
      setNoteBody('')
      setTaskForm(emptyContactTaskForm)
      setMode('detail')
      navigate(`/contacts/${contactID}`)
      return
    }

    try {
      const [data, taskData, dealData] = await Promise.all([
        getContact(contactID),
        listTasks({ status: 'open', entityType: 'contact', entityId: contactID }),
        listDeals({ primaryContactId: contactID })
      ])
      const detailData = { ...data, tasks: taskData.tasks || [], deals: dealData.deals || [] }
      setDetailCache((current) => ({ ...current, [contactID]: detailData }))
      setSelectedContactId(contactID)
      setDetail(detailData)
      setForm(contactFormValues(data.contact, customDefinitions))
      setNoteBody('')
      setTaskForm(emptyContactTaskForm)
      setMode('detail')
      navigate(`/contacts/${contactID}`)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load contact.')
    }
  }

  useEffect(() => {
    const controller = new AbortController()

    async function openRouteContact() {
      if (!customDefinitionsLoaded) return
      if (!Number.isInteger(routeContactId) || routeContactId <= 0) {
        if (selectedContactId || mode === 'detail') {
          setSelectedContactId(null)
          setDetail(null)
          setForm(emptyContactForm)
          setNoteBody('')
          setTaskForm(emptyContactTaskForm)
          setMode('list')
        }
        return
      }

      if (selectedContactId === routeContactId && detail?.contact?.id === routeContactId) {
        return
      }

      const cached = detailCache[routeContactId]
      if (cached) {
        setSelectedContactId(routeContactId)
        setDetail(cached)
        setForm(contactFormValues(cached.contact, customDefinitions))
        setNoteBody('')
        setTaskForm(emptyContactTaskForm)
        setMode('detail')
        setError('')
        return
      }

      try {
        setIsDetailLoading(true)
        const [data, taskData, dealData] = await Promise.all([
          getContact(routeContactId, { signal: controller.signal }),
          listTasks({ status: 'open', entityType: 'contact', entityId: routeContactId }, { signal: controller.signal }),
          listDeals({ primaryContactId: routeContactId }, { signal: controller.signal })
        ])
        if (controller.signal.aborted) {
          return
        }
        const detailData = { ...data, tasks: taskData.tasks || [], deals: dealData.deals || [] }
        setDetailCache((current) => ({ ...current, [routeContactId]: detailData }))
        setSelectedContactId(routeContactId)
        setDetail(detailData)
        setForm(contactFormValues(detailData.contact, customDefinitions))
        setNoteBody('')
        setTaskForm(emptyContactTaskForm)
        setMode('detail')
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load contact.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsDetailLoading(false)
        }
      }
    }

    openRouteContact()
    return () => {
      controller.abort()
    }
  }, [customDefinitionsLoaded, detail, detailCache, mode, routeContactId, selectedContactId])

  async function handleCreate(event) {
    event.preventDefault()
    try {
      const data = await createContact(contactPayload(form, customDefinitions))
      const detailData = { ...data, notes: data.notes || [], tasks: data.tasks || [], deals: [] }
      setDetailCache((current) => ({ ...current, [data.contact.id]: detailData }))
      setContacts((current) => [...current, data.contact])
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      setSelectedContactId(data.contact.id)
      setDetail(detailData)
      setForm(contactFormValues(detailData.contact, customDefinitions))
      setNoteBody('')
      setTaskForm(emptyContactTaskForm)
      setMode('detail')
      navigate(`/contacts/${data.contact.id}`)
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to create contact.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.email || `${form.firstName} ${form.lastName}`))
      setDuplicateCandidate(saveError.duplicate || null)
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    if (!selectedContactId) {
      return
    }

    try {
      const data = await updateContact(selectedContactId, contactPayload(form, customDefinitions))
      const detailData = {
        ...data,
        notes: detail?.notes || data.notes || [],
        tasks: detail?.tasks || data.tasks || [],
        deals: detail?.deals || []
      }
      setDetailCache((current) => ({ ...current, [selectedContactId]: detailData }))
      setContacts((current) => current.map((entry) => (entry.id === selectedContactId ? data.contact : entry)))
      setDetail(detailData)
      setForm(contactFormValues(data.contact, customDefinitions))
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to update contact.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.email || `${form.firstName} ${form.lastName}`))
      setDuplicateCandidate(saveError.duplicate || null)
    }
  }

  async function handleArchive() {
    if (!selectedContactId) {
      return
    }

    try {
      await archiveContact(selectedContactId)
      setContacts((current) => current.filter((entry) => entry.id !== selectedContactId))
      setMeta((current) => ({ ...current, total: Math.max(0, current.total - 1) }))
      setDetail((current) => {
        if (!current?.contact?.id) {
          return null
        }
        const next = { ...detailCache }
        delete next[current.contact.id]
        setDetailCache(next)
        return null
      })
      setSelectedContactId(null)
      setForm(emptyContactForm)
      setNoteBody('')
      setTaskForm(emptyContactTaskForm)
      setMode('list')
      navigate('/companies')
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive contact.')
    }
  }

  async function handleCreateNote(event) {
    event.preventDefault()
    if (!selectedContactId || !noteBody.trim()) {
      return
    }

    try {
      const data = await createNote({
        entityType: 'contact',
        entityId: selectedContactId,
        body: noteBody.trim()
      })
      setDetail((current) => {
        if (!current) {
          return current
        }
        const next = {
          ...current,
          notes: [data.note, ...(current.notes || [])],
          activities: [data.activity, ...(current.activities || [])]
        }
        setDetailCache((cache) => ({ ...cache, [selectedContactId]: next }))
        return next
      })
      setNoteBody('')
      setError('')
    } catch (noteError) {
      setError(noteError.message || 'Unable to add note.')
    }
  }

  async function handleToggleEmail() {
    const next = !emailOpen
    setEmailOpen(next)
    setEmailStatus('')
    if (next) {
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
      if (selectedContactId) {
        try {
          const history = await listEmailMessages({ entityType: 'contact', entityId: selectedContactId })
          setEmailHistory(history)
        } catch (historyError) {
          if (!isAbortError(historyError)) {
            setEmailHistory([])
          }
        }
      }
    }
  }

  async function handleToggleCalls() {
    const next = !callsOpen
    setCallsOpen(next)
    setCallStatus('')
    if (!next || !selectedContactId) {
      return
    }
    try {
      setCallLogs(await listCalls({ entityType: 'contact', entityId: selectedContactId }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load call history.')
      }
    }
  }

  async function handleToggleSMS() {
    const next = !smsOpen
    setSmsOpen(next)
    setSmsStatus('')
    if (!next || !selectedContactId) {
      return
    }
    try {
      setSmsMessages(await listSMSMessages({ entityType: 'contact', entityId: selectedContactId }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load SMS history.')
      }
    }
  }

  function applySMSTemplate(templateName) {
    const template = smsTemplates.find((entry) => entry.name === templateName)
    setSmsForm({ templateName, body: template?.body || '' })
  }

  async function handleSendSMS(event) {
    event.preventDefault()
    if (!selectedContactId || !selectedContact?.phone || !smsForm.body.trim()) {
      return
    }
    setIsSendingSMS(true)
    setSmsStatus('')
    try {
      const message = await sendContactSMS(selectedContactId, {
        body: smsForm.body.trim(),
        templateName: smsForm.templateName
      })
      setSmsMessages((current) => [message, ...current.filter((entry) => entry.id !== message.id)])
      setSmsOpen(true)
      setSmsForm(emptySMSForm)
      setSmsStatus(message.status === 'suppressed' ? 'SMS suppressed because this phone number is opted out.' : (message.status === 'failed' ? 'SMS failed.' : 'SMS sent.'))
      setError('')
    } catch (sendError) {
      setError(sendError.message || 'Unable to send SMS.')
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
    if (!selectedContactId || !selectedContact?.phone || !inboundSMSForm.body.trim()) {
      return
    }
    setIsLoggingInboundSMS(true)
    setSmsStatus('')
    try {
      const message = await logInboundSMS({
        entityType: 'contact',
        entityId: selectedContactId,
        phoneNumber: selectedContact.phone,
        body: inboundSMSForm.body.trim()
      })
      setSmsMessages((current) => [message, ...current.filter((entry) => entry.id !== message.id)])
      setSmsOpen(true)
      setInboundSMSOpen(false)
      setInboundSMSForm(emptyInboundSMSForm)
      setSmsStatus('Inbound SMS logged. STOP-style replies opt the number out automatically.')
      setError('')
    } catch (logError) {
      setError(logError.message || 'Unable to log inbound SMS.')
    } finally {
      setIsLoggingInboundSMS(false)
    }
  }

  async function handleSMSOptOut() {
    if (!selectedContactId || !selectedContact?.phone) {
      return
    }
    setIsOptingOutSMS(true)
    setSmsStatus('')
    try {
      await optOutSMS({
        phoneNumber: selectedContact.phone,
        reason: 'manual',
        source: 'contact_detail',
        entityType: 'contact',
        entityId: selectedContactId
      })
      setSmsStatus('SMS opt-out recorded.')
      setError('')
    } catch (optOutError) {
      setError(optOutError.message || 'Unable to opt out phone number.')
    } finally {
      setIsOptingOutSMS(false)
    }
  }

  async function handleToggleMeetings() {
    const next = !meetingsOpen
    setMeetingsOpen(next)
    setMeetingStatus('')
    if (!next || !selectedContactId) {
      return
    }
    try {
      setMeetingEvents(await listCalendarEvents({ entityType: 'contact', entityId: selectedContactId }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load meetings.')
      }
    }
  }

  async function handleScheduleMeeting(event) {
    event.preventDefault()
    if (!selectedContactId || !meetingForm.title.trim()) {
      return
    }
    const startAt = localDateTimeToISOString(meetingForm.startAt)
    const endAt = localDateTimeToISOString(meetingForm.endAt)
    if (!startAt || !endAt) {
      setError('Meeting start and end are required.')
      return
    }
    setIsSchedulingMeeting(true)
    setMeetingStatus('')
    try {
      const meeting = await scheduleCalendarEvent({
        entityType: 'contact',
        entityId: selectedContactId,
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
      setError('')
    } catch (scheduleError) {
      setError(scheduleError.message || 'Unable to schedule meeting.')
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
      setError('')
    } catch (cancelError) {
      setError(cancelError.message || 'Unable to cancel meeting.')
    } finally {
      setCancellingMeetingId(null)
    }
  }

  async function handleStartCall() {
    if (!selectedContactId || !selectedContact?.phone) {
      return
    }
    setIsStartingCall(true)
    setCallStatus('')
    try {
      const result = await startCall({ entityType: 'contact', entityId: selectedContactId, phoneNumber: selectedContact.phone })
      if (result?.call) {
        setActiveCall(result.call)
        setCallLogs((current) => [result.call, ...current.filter((call) => call.id !== result.call.id)])
      }
      setCallDialURL(result?.dialUrl || '')
      setCallsOpen(true)
      setCallStatus('Call started. Log the outcome when you finish.')
      setError('')
    } catch (startError) {
      setError(startError.message || 'Unable to start call.')
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
      setError('')
    } catch (completeError) {
      setError(completeError.message || 'Unable to log call outcome.')
    } finally {
      setIsCompletingCall(false)
    }
  }

  function handleToggleInboundCallLog() {
    setInboundCallOpen((current) => {
      const next = !current
      if (next) {
        setManualCallForm((form) => ({ ...form, phoneNumber: form.phoneNumber || selectedContact?.phone || '' }))
      }
      return next
    })
    setCallStatus('')
  }

  async function handleRecordInboundCall(event) {
    event.preventDefault()
    if (!selectedContactId || !manualCallForm.phoneNumber.trim()) {
      return
    }
    setIsLoggingCall(true)
    setCallStatus('')
    try {
      const call = await logCall({
        entityType: 'contact',
        entityId: selectedContactId,
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
      setError('')
    } catch (logError) {
      setError(logError.message || 'Unable to log inbound call.')
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
      setError('')
    } catch (recordingError) {
      setError(recordingError.message || 'Unable to update call recording.')
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
      setError('')
    } catch (recordingError) {
      setError(recordingError.message || 'Unable to delete call recording.')
    } finally {
      setIsUpdatingRecording(false)
    }
  }

  async function handleToggleSequences() {
    const next = !sequencesOpen
    setSequencesOpen(next)
    setSequenceStatus('')
    if (!next || !selectedContactId) {
      return
    }
    try {
      const [sequences, enrollments] = await Promise.all([
        listEmailSequences(),
        listEmailSequenceEnrollments({ contactId: selectedContactId })
      ])
      setSequenceOptions(sequences)
      setSequenceEnrollments(enrollments)
      setSequenceForm((current) => ({ sequenceId: current.sequenceId || (sequences[0]?.id ? String(sequences[0].id) : '') }))
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load email sequences.')
      }
    }
  }

  async function handleEnrollSequence(event) {
    event.preventDefault()
    if (!selectedContactId || !sequenceForm.sequenceId) {
      return
    }
    setIsEnrollingSequence(true)
    setSequenceStatus('')
    try {
      const enrollment = await createEmailSequenceEnrollment({
        contactId: selectedContactId,
        sequenceId: Number.parseInt(sequenceForm.sequenceId, 10)
      })
      setSequenceEnrollments((current) => [enrollment, ...current.filter((entry) => entry.id !== enrollment.id)])
      setSequenceStatus(`Enrolled in ${enrollment.sequenceName || 'sequence'}.`)
      setError('')
    } catch (enrollError) {
      setError(enrollError.message || 'Unable to enroll contact in sequence.')
    } finally {
      setIsEnrollingSequence(false)
    }
  }

  async function handleCancelSequenceEnrollment(enrollmentId) {
    try {
      await cancelEmailSequenceEnrollment(enrollmentId)
      setSequenceEnrollments((current) => current.filter((entry) => entry.id !== enrollmentId))
      setSequenceStatus('Sequence enrollment cancelled.')
      setError('')
    } catch (cancelError) {
      setError(cancelError.message || 'Unable to cancel sequence enrollment.')
    }
  }

  async function handleEvaluateLeadScore() {
    if (!selectedContactId) {
      return
    }
    const contactKey = selectedContactId
    setIsEvaluatingLeadScore(true)
    setLeadScoreStatus('')
    try {
      const evaluation = await evaluateContactLeadScore(contactKey)
      const scoredContact = evaluation?.contact
      if (scoredContact) {
        setDetail((current) => {
          if (!current) return current
          const next = { ...current, contact: scoredContact }
          setDetailCache((cache) => ({ ...cache, [contactKey]: next }))
          return next
        })
        setContacts((current) => current.map((entry) => (entry.id === scoredContact.id ? scoredContact : entry)))
      }
      const matchedCount = evaluation?.matchedRules?.length || 0
      const gradeText = evaluation?.grade ? ` (${evaluation.grade})` : ''
      const assignmentText = evaluation?.assignedToUserName ? ` Routed to ${evaluation.assignedToUserName}.` : ''
      setLeadScoreStatus(`Lead scored ${evaluation?.score ?? 0}${gradeText}; ${matchedCount} rule${matchedCount === 1 ? '' : 's'} matched.${assignmentText}`)
      setError('')
    } catch (scoreError) {
      setError(scoreError.message || 'Unable to evaluate lead score.')
    } finally {
      setIsEvaluatingLeadScore(false)
    }
  }

  function applyEmailTemplate(templateId) {
    const template = emailTemplates.find((item) => String(item.id) === String(templateId))
    if (template) {
      setEmailForm({ subject: template.subject, body: template.body })
    }
  }

  async function handleSendEmail(event) {
    event.preventDefault()
    if (!selectedContactId) {
      return
    }
    setIsSendingEmail(true)
    setEmailStatus('')
    try {
      const result = await sendContactEmail(selectedContactId, emailForm)
      setEmailStatus(`Email sent to ${result?.to || 'contact'}.`)
      setEmailForm({ subject: '', body: '' })
      setError('')
      try {
        const history = await listEmailMessages({ entityType: 'contact', entityId: selectedContactId })
        setEmailHistory(history)
      } catch (historyError) {
        if (!isAbortError(historyError)) {
          // history refresh is best-effort
        }
      }
    } catch (sendError) {
      setError(sendError.message || 'Unable to send email.')
    } finally {
      setIsSendingEmail(false)
    }
  }

  async function handleCreateTask(event) {
    event.preventDefault()
    if (!selectedContactId || !taskForm.title.trim()) {
      return
    }

    try {
      const data = await createTask({
        entityType: 'contact',
        entityId: selectedContactId,
        title: taskForm.title.trim(),
        description: taskForm.description.trim(),
        status: 'open',
        dueAt: taskForm.dueAt ? `${taskForm.dueAt}:00Z` : '',
        assignedToUserId: Number.parseInt(taskForm.assignedToUserId, 10) || 0
      })
      setDetail((current) => {
        if (!current) {
          return current
        }
        const next = {
          ...current,
          tasks: [data.task, ...(current.tasks || []).filter((task) => task.id !== data.task.id)],
          activities: [...(data.activities || []), ...(current.activities || [])]
        }
        setDetailCache((cache) => ({ ...cache, [selectedContactId]: next }))
        return next
      })
      setTaskForm(emptyContactTaskForm)
      setError('')
    } catch (taskError) {
      setError(taskError.message || 'Unable to create task.')
    }
  }

  const detailTitle = useMemo(() => fullContactName(selectedContact || {}), [selectedContact])

  return (
    <section className="dashboard-grid contacts-grid">
      <ContactListCard
        bulkActions={{ entityType: 'contact', selectedIds: selectedContactIds, visibleIds: contacts.map((contact) => contact.id), onSelectionChange: setSelectedContactIds, onChanged: () => reloadContacts(search, ownerFilter), statuses: bulkStatusOptions.contact, userOptions }}
        canWrite={canWrite}
        contacts={contacts}
        currentUserId={currentUserId}
        customDefinitions={customDefinitions}
        customFilter={customFilter}
        duplicateCandidate={duplicateCandidate}
        duplicateSearch={duplicateSearch}
        error={error}
        exportURL={contactsExportURL({ search, customField: customFilter })}
        hasFilter={hasFilter}
        isLoading={isListLoading}
        meta={meta}
        onApplyOwnerFilter={applyOwnerFilter}
        onApplyCustomFilter={applyCustomFilter}
        onApplySavedView={handleApplySavedView}
        onClearFilters={() => {
          setSearch('')
          setOwnerFilter('all')
          setCustomFilter({ fieldKey: '', operator: '', value: '' })
          navigate(buildContactsPath('', 'all', { fieldKey: '', operator: '', value: '' }), { replace: true })
          reloadContacts('', 'all', { fieldKey: '', operator: '', value: '' })
        }}
        onClearCustomFilter={() => applyCustomFilter({ fieldKey: '', operator: '', value: '' })}
        onCreate={() => {
          navigate('/contacts')
          setMode('create')
          setForm(emptyContactForm)
          setDetail(null)
          setSelectedContactId(null)
        }}
        onDuplicateSearch={handleDuplicateSearch}
        onOpenContact={handleOpenContact}
        onOpenDuplicate={handleOpenDuplicate}
        onOwnerFilterChange={handleOwnerFilterChange}
        onReload={() => reloadContacts(search)}
        onSearchChange={handleSearchChange}
        ownerFilter={ownerFilter}
        search={search}
        userOptions={userOptions}
      />

      {canWrite && mode === 'create' ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>New contact</h2>
              <p>Add the next person you need to move through the pipeline.</p>
            </div>
            <ContactForm customDefinitions={customDefinitions} form={form} onSetForm={setForm} onSubmit={handleCreate} submitLabel="Save contact" />
          </div>
        </Card>
      ) : null}

      {mode === 'detail' && selectedContact ? (
        <Card>
          <div className="card-stack">
            {isDetailLoading ? <p className="field-hint">Loading contact detail...</p> : null}
            <div className="section-header">
              <div>
                <h2>{detailTitle}</h2>
                <p>{selectedContact.email || formatContactAddress(selectedContact) || selectedContact.phone}</p>
              </div>
              {canWrite ? (
                <Button className="button-danger" onClick={handleArchive}>
                  Archive contact
                </Button>
              ) : null}
            </div>
            <ContactLeadScoreCard
              canWrite={canWrite}
              contact={selectedContact}
              isEvaluating={isEvaluatingLeadScore}
              onEvaluate={handleEvaluateLeadScore}
              status={leadScoreStatus}
            />
            <ContactForm
              canSubmit={canWrite}
              customDefinitions={customDefinitions}
              form={form}
              includeStatus
              onSetForm={setForm}
              onSubmit={handleUpdate}
              submitLabel="Update contact"
            />
            <ContactAttributionCard contact={selectedContact} />
            <ClientAccountContext canWrite={canWrite} deals={selectedDeals}
              isCustomer={selectedContact.isClient || selectedContact.status === 'customer'} labels={pipelineLabels}
              notes={selectedNotes} onCreateDeal={handleCreateRelatedDeal} onOpenDeal={handleOpenDeal} tasks={selectedTasks}
            />
            <ContactCallsCard
              activeCall={activeCall}
              callForm={callForm}
              calls={callLogs}
              canWrite={canWrite}
              contact={selectedContact}
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
              contact={selectedContact}
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
            <ContactEmailCard
              canWrite={canWrite}
              form={emailForm}
              history={emailHistory}
              isSending={isSendingEmail}
              onApplyTemplate={applyEmailTemplate}
              onSend={handleSendEmail}
              onSetForm={setEmailForm}
              onToggle={handleToggleEmail}
              open={emailOpen}
              status={emailStatus}
              templates={emailTemplates}
            />
            <ContactSequencesCard
              canWrite={canWrite}
              enrollments={sequenceEnrollments}
              form={sequenceForm}
              isEnrolling={isEnrollingSequence}
              onCancel={handleCancelSequenceEnrollment}
              onEnroll={handleEnrollSequence}
              onSetForm={setSequenceForm}
              onToggle={handleToggleSequences}
              open={sequencesOpen}
              options={sequenceOptions}
              status={sequenceStatus}
            />
            <TouchpointSummary entityType="contact" entityId={selectedContactId} refreshKey={JSON.stringify({ selectedActivities, selectedNotes, selectedTasks, emailHistory, smsMessages, meetingEvents, callLogs })} />
            <RecordWorkCards
              activities={selectedActivities}
              canWrite={canWrite}
              entityId={selectedContactId}
              entityType="contact"
              noteBody={noteBody}
              notes={selectedNotes}
              onCreateNote={handleCreateNote}
              onCreateTask={handleCreateTask}
              onOpenTasks={handleOpenContactTasks}
              onSetNoteBody={setNoteBody}
              onSetTaskForm={setTaskForm}
              taskForm={taskForm}
              tasks={selectedTasks}
              users={userOptions}
            />
          </div>
        </Card>
      ) : null}
    </section>
  )
}

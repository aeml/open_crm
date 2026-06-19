import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { EmptyState } from '../components/ui/empty_state'
import { SavedViews } from '../components/ui/saved_views'
import { ActivityTimeline } from '../components/ui/activity_timeline'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { completeCall, listCalls, startCall } from '../lib/calls'
import { archiveContact, contactsExportURL, createContact, getContact, listContacts, sendContactEmail, updateContact } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { createNote } from '../lib/notes'
import { listEmailSequences } from '../lib/email_sequences'
import { cancelEmailSequenceEnrollment, createEmailSequenceEnrollment, listEmailSequenceEnrollments } from '../lib/email_sequence_enrollments'
import { listEmailTemplates } from '../lib/email_templates'
import { listEmailMessages } from '../lib/email_messages'
import { createTask, listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = {
  firstName: '',
  lastName: '',
  email: '',
  phone: '',
  addressLine1: '',
  addressLine2: '',
  city: '',
  state: '',
  postalCode: '',
  country: '',
  jobTitle: '',
  status: 'lead'
}

function formatAddress(contact = {}) {
  const street = [contact.addressLine1, contact.addressLine2].filter(Boolean).join(', ')
  const locality = [contact.city, contact.state, contact.postalCode].filter(Boolean).join(', ')
  return [street, locality, contact.country].filter(Boolean).join(' | ')
}

const emptyTaskForm = {
  title: '',
  description: '',
  dueAt: '',
  assignedToUserId: ''
}

const emptyCallForm = {
  disposition: '',
  notes: ''
}

function fullName(contact) {
  return `${contact.firstName || ''} ${contact.lastName || ''}`.trim()
}

function contactFormValues(contact) {
  return {
    firstName: contact.firstName || '',
    lastName: contact.lastName || '',
    email: contact.email || '',
    phone: contact.phone || '',
    addressLine1: contact.addressLine1 || '',
    addressLine2: contact.addressLine2 || '',
    city: contact.city || '',
    state: contact.state || '',
    postalCode: contact.postalCode || '',
    country: contact.country || '',
    jobTitle: contact.jobTitle || '',
    status: contact.status || 'lead'
  }
}

function duplicateSearchTerm(message, fallback = '') {
  const text = String(message || '')
  const marker = text.toLowerCase().lastIndexOf('duplicate contact:')
  if (marker >= 0) {
    const candidate = text.slice(marker + 'duplicate contact:'.length).split('(')[0].trim()
    if (candidate) {
      return candidate
    }
  }
  return String(fallback || '').trim()
}

function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD' }).format(amount)
}

function formatSequenceTime(value) {
  if (!value) {
    return 'Not scheduled'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not scheduled' : date.toLocaleString()
}

function formatCallTime(value) {
  if (!value) {
    return 'Unknown time'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Unknown time' : date.toLocaleString()
}

function relatedPipelineLabels(businessType) {
  if (businessType === 'services' || businessType === 'construction-services') {
    return { plural: 'Jobs', singular: 'job' }
  }
  if (businessType === 'product-sales') {
    return { plural: 'Opportunities', singular: 'opportunity' }
  }
  return { plural: 'Deals', singular: 'deal' }
}

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
  const [mode, setMode] = useState('list')
  const [contacts, setContacts] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState(initialSearch)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [selectedContactId, setSelectedContactId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [userOptions, setUserOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm)
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
  const [callsOpen, setCallsOpen] = useState(false)
  const [callLogs, setCallLogs] = useState([])
  const [activeCall, setActiveCall] = useState(null)
  const [callDialURL, setCallDialURL] = useState('')
  const [callForm, setCallForm] = useState(emptyCallForm)
  const [callStatus, setCallStatus] = useState('')
  const [isStartingCall, setIsStartingCall] = useState(false)
  const [isCompletingCall, setIsCompletingCall] = useState(false)
  const [sequencesOpen, setSequencesOpen] = useState(false)
  const [sequenceOptions, setSequenceOptions] = useState([])
  const [sequenceEnrollments, setSequenceEnrollments] = useState([])
  const [sequenceForm, setSequenceForm] = useState({ sequenceId: '' })
  const [sequenceStatus, setSequenceStatus] = useState('')
  const [isEnrollingSequence, setIsEnrollingSequence] = useState(false)
  const searchControllerRef = useRef(null)

  const selectedContact = detail?.contact || null
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedDeals = detail?.deals || []
  const hasFilter = search.trim() !== '' || ownerFilter !== 'all'
  const selectedActivities = detail?.activities || []

  useEffect(() => {
    setCallsOpen(false)
    setCallLogs([])
    setActiveCall(null)
    setCallDialURL('')
    setCallForm(emptyCallForm)
    setCallStatus('')
    setSequencesOpen(false)
    setSequenceEnrollments([])
    setSequenceStatus('')
    setSequenceForm({ sequenceId: '' })
  }, [selectedContactId])

  function buildContactsPath(nextSearch = search, nextOwner = ownerFilter) {
    const params = new URLSearchParams()
    if (nextSearch) {
      params.set('q', nextSearch)
    }
    if (nextOwner !== 'all') {
      params.set('owner', nextOwner)
    }
    const suffix = params.toString() ? `?${params.toString()}` : ''
    return `/contacts${suffix}`
  }

  async function loadContacts(nextSearch = '', nextOwner = 'all', { signal } = {}) {
    const isUnassigned = nextOwner === 'unassigned'
    const ownerUserId = isUnassigned || nextOwner === 'all' ? 0 : Number.parseInt(nextOwner, 10) || 0
    const data = await listContacts({ search: nextSearch, unassigned: isUnassigned, ownerUserId }, { signal })

    if (Array.isArray(data?.contacts)) {
      setContacts(data.contacts)
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

  useEffect(() => {
    const controller = new AbortController()

    async function run() {
      setIsListLoading(true)
      try {
        await Promise.all([loadContacts(initialSearch, initialOwnerFilter, { signal: controller.signal }), loadUserOptions({ signal: controller.signal })])
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
    setSearch(nextSearch)
    setOwnerFilter(nextOwner)
    setMode('list')
    setDetail(null)
    setSelectedContactId(null)
    navigate(buildContactsPath(nextSearch, nextOwner), { replace: true })
    await reloadContacts(nextSearch, nextOwner)
  }

  async function reloadContacts(nextSearch = search, nextOwner = ownerFilter) {
    searchControllerRef.current?.abort()
    const controller = new AbortController()
    searchControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadContacts(nextSearch, nextOwner, { signal: controller.signal })
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
      setForm(contactFormValues(cached.contact))
      setNoteBody('')
      setTaskForm(emptyTaskForm)
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
      setForm(contactFormValues(data.contact))
      setNoteBody('')
      setTaskForm(emptyTaskForm)
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
      if (!Number.isInteger(routeContactId) || routeContactId <= 0) {
        if (selectedContactId || mode === 'detail') {
          setSelectedContactId(null)
          setDetail(null)
          setForm(emptyForm)
          setNoteBody('')
          setTaskForm(emptyTaskForm)
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
        setForm(contactFormValues(cached.contact))
        setNoteBody('')
        setTaskForm(emptyTaskForm)
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
        setForm(contactFormValues(detailData.contact))
        setNoteBody('')
        setTaskForm(emptyTaskForm)
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
  }, [detail, detailCache, mode, routeContactId, selectedContactId])

  async function handleCreate(event) {
    event.preventDefault()
    try {
      const data = await createContact(form)
      const detailData = { ...data, notes: data.notes || [], tasks: data.tasks || [], deals: [] }
      setDetailCache((current) => ({ ...current, [data.contact.id]: detailData }))
      setContacts((current) => [...current, data.contact])
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      setSelectedContactId(data.contact.id)
      setDetail(detailData)
      setForm(contactFormValues(detailData.contact))
      setNoteBody('')
      setTaskForm(emptyTaskForm)
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
      const data = await updateContact(selectedContactId, form)
      const detailData = {
        ...data,
        notes: detail?.notes || data.notes || [],
        tasks: detail?.tasks || data.tasks || [],
        deals: detail?.deals || []
      }
      setDetailCache((current) => ({ ...current, [selectedContactId]: detailData }))
      setContacts((current) => current.map((entry) => (entry.id === selectedContactId ? data.contact : entry)))
      setDetail(detailData)
      setForm(contactFormValues(data.contact))
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
      setForm(emptyForm)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
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
      setTaskForm(emptyTaskForm)
      setError('')
    } catch (taskError) {
      setError(taskError.message || 'Unable to create task.')
    }
  }

  const detailTitle = useMemo(() => fullName(selectedContact || {}), [selectedContact])

  return (
    <section className="dashboard-grid contacts-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Contacts</h2>
              <p>Keep the right people moving without a bloated CRM mess.</p>
            </div>
            <div className="button-row">
              <a className="button button-secondary" href={contactsExportURL(search)}>
                Export CSV
              </a>
              {canWrite ? (
                <Button
                  onClick={() => {
                    navigate('/contacts')
                    setMode('create')
                    setForm(emptyForm)
                    setDetail(null)
                    setSelectedContactId(null)
                  }}
                >
                  Add contact
                </Button>
              ) : null}
            </div>
          </div>
          <Field label="Search contacts">
            <input className="text-input" type="search" value={search} onChange={handleSearchChange} />
          </Field>
          <SavedViews entityType="contacts" currentFilters={{ q: search, owner: ownerFilter }} onApply={handleApplySavedView} defaultName="Contact view" />
          <Field label="Owner filter">
            <div className="button-row">
              <select className="text-input" value={ownerFilter} onChange={handleOwnerFilterChange}>
                <option value="all">All owners</option>
                <option value="unassigned">Unassigned</option>
                {userOptions.map((user) => (
                  <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                ))}
              </select>
              {currentUserId ? (
                <Button className={ownerFilter === currentUserId ? '' : 'button-secondary'} type="button" onClick={() => applyOwnerFilter(currentUserId)}>
                  Mine
                </Button>
              ) : null}
              <Button className={ownerFilter === 'unassigned' ? '' : 'button-secondary'} type="button" onClick={() => applyOwnerFilter('unassigned')}>
                Unassigned
              </Button>
            </div>
          </Field>
          {isListLoading ? <p className="field-hint">Loading contacts...</p> : null}
          {error ? (
            <div className="card-stack">
              <p className="form-error">{error}</p>
              <div>
                <Button className="button-secondary" type="button" onClick={() => reloadContacts(search)}>
                  Retry contacts
                </Button>
              </div>
              {duplicateCandidate ? (
                <div>
                  <Button className="button-secondary" onClick={handleOpenDuplicate}>
                    Open matching contact
                  </Button>
                </div>
              ) : null}
              {duplicateSearch ? (
                <div>
                  <Button className="button-secondary" onClick={handleDuplicateSearch}>
                    Search existing contacts for {duplicateSearch}
                  </Button>
                </div>
              ) : null}
            </div>
          ) : null}
          <div className="record-list" role="list" aria-label="Contacts list">
            {!isListLoading && contacts.length === 0 ? (
              <EmptyState
                title={hasFilter ? 'No contacts match the current filters.' : 'No contacts yet.'}
                description={hasFilter ? 'Try a different name, email, phone number, or change the owner filter.' : 'Add the first person you need to follow up with. You can link contacts to clients, deals, notes, and tasks later.'}
                actionLabel={hasFilter ? 'Clear filters' : (canWrite ? 'Create first contact' : '')}
                onAction={() => {
                  if (hasFilter) {
                    setSearch('')
                    setOwnerFilter('all')
                    navigate(buildContactsPath('', 'all'), { replace: true })
                    reloadContacts('', 'all')
                    return
                  }
                  navigate('/contacts')
                  setMode('create')
                  setForm(emptyForm)
                  setDetail(null)
                  setSelectedContactId(null)
                }}
              />
            ) : contacts.map((contact) => (
              <article className="record-row" key={contact.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenContact(contact)}>
                    {fullName(contact)}
                  </button>
                  <p>{contact.jobTitle || 'No title'}</p>
                </div>
                <div>
                  <p>{contact.email || formatAddress(contact) || 'No contact details'}</p>
                  <p>{contact.status}</p>
                  <p className="field-hint">{contact.ownerUserName || 'Unassigned'}</p>
                </div>
              </article>
            ))}
          </div>
          <p className="field-hint">Showing {contacts.length} of {meta.total} contacts.</p>
        </div>
      </Card>

      {canWrite && mode === 'create' ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>New contact</h2>
              <p>Add the next person you need to move through the pipeline.</p>
            </div>
            <form className="auth-form" onSubmit={handleCreate}>
              <Field label="First name">
                <input className="text-input" value={form.firstName} onChange={(event) => setForm((current) => ({ ...current, firstName: event.target.value }))} required />
              </Field>
              <Field label="Last name">
                <input className="text-input" value={form.lastName} onChange={(event) => setForm((current) => ({ ...current, lastName: event.target.value }))} required />
              </Field>
              <Field label="Email">
                <input className="text-input" type="email" value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} />
              </Field>
              <Field label="Phone">
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              <Field label="Address line 1">
                <input className="text-input" value={form.addressLine1} onChange={(event) => setForm((current) => ({ ...current, addressLine1: event.target.value }))} />
              </Field>
              <Field label="Address line 2">
                <input className="text-input" value={form.addressLine2} onChange={(event) => setForm((current) => ({ ...current, addressLine2: event.target.value }))} />
              </Field>
              <Field label="City">
                <input className="text-input" value={form.city} onChange={(event) => setForm((current) => ({ ...current, city: event.target.value }))} />
              </Field>
              <Field label="State">
                <input className="text-input" value={form.state} onChange={(event) => setForm((current) => ({ ...current, state: event.target.value }))} />
              </Field>
              <Field label="Postal code">
                <input className="text-input" value={form.postalCode} onChange={(event) => setForm((current) => ({ ...current, postalCode: event.target.value }))} />
              </Field>
              <Field label="Country">
                <input className="text-input" value={form.country} onChange={(event) => setForm((current) => ({ ...current, country: event.target.value }))} />
              </Field>
              <Field label="Job title">
                <input className="text-input" value={form.jobTitle} onChange={(event) => setForm((current) => ({ ...current, jobTitle: event.target.value }))} />
              </Field>
              <Button type="submit">Save contact</Button>
            </form>
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
                <p>{selectedContact.email || formatAddress(selectedContact) || selectedContact.phone}</p>
              </div>
              {canWrite ? (
                <Button className="button-danger" onClick={handleArchive}>
                  Archive contact
                </Button>
              ) : null}
            </div>
            <form className="auth-form" onSubmit={handleUpdate}>
              <Field label="First name">
                <input className="text-input" value={form.firstName} onChange={(event) => setForm((current) => ({ ...current, firstName: event.target.value }))} required />
              </Field>
              <Field label="Last name">
                <input className="text-input" value={form.lastName} onChange={(event) => setForm((current) => ({ ...current, lastName: event.target.value }))} required />
              </Field>
              <Field label="Email">
                <input className="text-input" type="email" value={form.email} onChange={(event) => setForm((current) => ({ ...current, email: event.target.value }))} />
              </Field>
              <Field label="Phone">
                <input className="text-input" value={form.phone} onChange={(event) => setForm((current) => ({ ...current, phone: event.target.value }))} />
              </Field>
              <Field label="Address line 1">
                <input className="text-input" value={form.addressLine1} onChange={(event) => setForm((current) => ({ ...current, addressLine1: event.target.value }))} />
              </Field>
              <Field label="Address line 2">
                <input className="text-input" value={form.addressLine2} onChange={(event) => setForm((current) => ({ ...current, addressLine2: event.target.value }))} />
              </Field>
              <Field label="City">
                <input className="text-input" value={form.city} onChange={(event) => setForm((current) => ({ ...current, city: event.target.value }))} />
              </Field>
              <Field label="State">
                <input className="text-input" value={form.state} onChange={(event) => setForm((current) => ({ ...current, state: event.target.value }))} />
              </Field>
              <Field label="Postal code">
                <input className="text-input" value={form.postalCode} onChange={(event) => setForm((current) => ({ ...current, postalCode: event.target.value }))} />
              </Field>
              <Field label="Country">
                <input className="text-input" value={form.country} onChange={(event) => setForm((current) => ({ ...current, country: event.target.value }))} />
              </Field>
              <Field label="Job title">
                <input className="text-input" value={form.jobTitle} onChange={(event) => setForm((current) => ({ ...current, jobTitle: event.target.value }))} />
              </Field>
              <Field label="Status">
                <select className="text-input" value={form.status} onChange={(event) => setForm((current) => ({ ...current, status: event.target.value }))}>
                  <option value="lead">Lead</option>
                  <option value="customer">Customer</option>
                  <option value="prospect">Prospect</option>
                </select>
              </Field>
              {canWrite ? <Button type="submit">Update contact</Button> : null}
            </form>
            <Card>
              <div className="card-stack">
                <div className="section-header">
                  <div>
                    <h3>{`Related ${pipelineLabels.plural.toLowerCase()}`}</h3>
                    <p>{`See active ${pipelineLabels.plural.toLowerCase()} tied to this contact.`}</p>
                  </div>
                  {canWrite ? (
                    <Button className="button-secondary" onClick={handleCreateRelatedDeal}>
                      {`Create ${pipelineLabels.singular}`}
                    </Button>
                  ) : null}
                </div>
                <div className="record-list" role="list" aria-label="Related deals list">
                  {selectedDeals.length === 0 ? (
                    <article className="record-row" role="listitem">
                      <div>
                        <p>{`No related ${pipelineLabels.plural.toLowerCase()} yet.`}</p>
                      </div>
                    </article>
                  ) : selectedDeals.map((deal) => (
                    <article className="record-row" key={deal.id} role="listitem">
                      <div>
                        <button className="button button-ghost contact-link" type="button" onClick={() => handleOpenDeal(deal.id)}>
                          {deal.name}
                        </button>
                        <p>{deal.stageName || deal.status || 'Unstaged'}</p>
                      </div>
                      <div>
                        <p>{formatMoney(deal.valueAmount, deal.valueCurrency)}</p>
                        <p>{deal.companyName || (deal.expectedCloseDate ? `Target ${deal.expectedCloseDate}` : 'No client linked')}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <div className="section-header">
                  <div>
                    <h3>Calls</h3>
                    <p className="field-hint">Start an outbound call and log the outcome on this contact.</p>
                  </div>
                  <div className="button-row">
                    <Button className="button-secondary" type="button" onClick={handleToggleCalls}>
                      {callsOpen ? 'Hide calls' : 'Show calls'}
                    </Button>
                    {canWrite && selectedContact.phone ? (
                      <Button className="button-secondary" type="button" onClick={handleStartCall} disabled={isStartingCall}>
                        {isStartingCall ? 'Starting...' : 'Start call'}
                      </Button>
                    ) : null}
                  </div>
                </div>
                {!selectedContact.phone ? <p className="field-hint">Add a phone number to this contact before starting a call.</p> : null}
                {callStatus ? <p className="field-hint" role="status">{callStatus}</p> : null}
                {callDialURL ? <a className="button button-ghost" href={callDialURL}>Open dialer</a> : null}
                {activeCall ? (
                  <form className="auth-form" onSubmit={handleCompleteCall}>
                    <Field label="Disposition">
                      <input className="text-input" value={callForm.disposition} onChange={(event) => setCallForm((current) => ({ ...current, disposition: event.target.value }))} placeholder="Connected, left voicemail, no answer" />
                    </Field>
                    <Field label="Call notes">
                      <textarea className="text-input" rows={4} value={callForm.notes} onChange={(event) => setCallForm((current) => ({ ...current, notes: event.target.value }))} />
                    </Field>
                    <Button type="submit" disabled={isCompletingCall}>{isCompletingCall ? 'Logging...' : 'Log call outcome'}</Button>
                  </form>
                ) : null}
                {callsOpen ? (
                  <div className="record-list" role="list" aria-label="Call history">
                    {callLogs.length === 0 ? (
                      <article className="record-row" role="listitem">
                        <div>
                          <p>No calls logged yet.</p>
                        </div>
                      </article>
                    ) : callLogs.map((call) => (
                      <article className="record-row" key={call.id} role="listitem">
                        <div>
                          <p>{call.disposition || (call.status === 'initiated' ? 'Call started' : 'Call logged')}</p>
                          <p className="field-hint">{call.phoneNumber} · {call.status}</p>
                          {call.notes ? <p className="field-hint">{call.notes}</p> : null}
                        </div>
                        <div>
                          <p>{formatCallTime(call.completedAt || call.startedAt || call.createdAt)}</p>
                          <p className="field-hint">{call.createdByUserName || 'You'}</p>
                        </div>
                      </article>
                    ))}
                  </div>
                ) : null}
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <div className="section-header">
                  <h3>Email</h3>
                  {canWrite ? (
                    <Button className="button-secondary" type="button" onClick={handleToggleEmail}>
                      {emailOpen ? 'Close' : 'Send email'}
                    </Button>
                  ) : null}
                </div>
                {emailStatus ? <p className="field-hint" role="status">{emailStatus}</p> : null}
                {canWrite && emailOpen ? (
                  <form className="auth-form" onSubmit={handleSendEmail}>
                    {emailTemplates.length > 0 ? (
                      <Field label="Template">
                        <select className="text-input" defaultValue="" onChange={(event) => applyEmailTemplate(event.target.value)}>
                          <option value="">Start from scratch</option>
                          {emailTemplates.map((template) => (
                            <option key={template.id} value={template.id}>{template.name}</option>
                          ))}
                        </select>
                      </Field>
                    ) : null}
                    <Field label="Subject">
                      <input className="text-input" value={emailForm.subject} onChange={(event) => setEmailForm({ ...emailForm, subject: event.target.value })} required />
                    </Field>
                    <Field label="Body">
                      <textarea className="text-input" rows={6} value={emailForm.body} onChange={(event) => setEmailForm({ ...emailForm, body: event.target.value })} required />
                    </Field>
                    <p className="field-hint">Merge fields like {'{{first_name}}'} are filled in when the email is sent.</p>
                    <Button type="submit" disabled={isSendingEmail}>{isSendingEmail ? 'Sending...' : 'Send email'}</Button>
                  </form>
                ) : null}
                {emailOpen && emailHistory.length > 0 ? (
                  <div className="record-list" role="list" aria-label="Email history">
                    {emailHistory.map((message) => (
                      <article className="record-row" key={message.id} role="listitem">
                        <div>
                          <p>{message.subject}</p>
                          <p className="field-hint">{message.status === 'failed' ? 'Failed' : 'Sent'} · {message.sentByName || 'You'}</p>
                        </div>
                      </article>
                    ))}
                  </div>
                ) : null}
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <div className="section-header">
                  <div>
                    <h3>Sequences</h3>
                    <p className="field-hint">Enroll this contact into a prepared cadence. Automated sending is not active yet.</p>
                  </div>
                  {canWrite ? (
                    <Button className="button-secondary" type="button" onClick={handleToggleSequences}>
                      {sequencesOpen ? 'Close' : 'Manage sequences'}
                    </Button>
                  ) : null}
                </div>
                {sequenceStatus ? <p className="field-hint" role="status">{sequenceStatus}</p> : null}
                {canWrite && sequencesOpen ? (
                  <form className="auth-form" onSubmit={handleEnrollSequence}>
                    {sequenceOptions.length > 0 ? (
                      <Field label="Sequence">
                        <select className="text-input" value={sequenceForm.sequenceId} onChange={(event) => setSequenceForm({ sequenceId: event.target.value })} required>
                          {sequenceOptions.map((sequence) => (
                            <option key={sequence.id} value={sequence.id}>{sequence.name}</option>
                          ))}
                        </select>
                      </Field>
                    ) : (
                      <p className="field-hint">Create a sequence in Settings, Email Sequences before enrolling contacts.</p>
                    )}
                    <Button type="submit" disabled={isEnrollingSequence || sequenceOptions.length === 0}>{isEnrollingSequence ? 'Enrolling...' : 'Enroll contact'}</Button>
                  </form>
                ) : null}
                {sequencesOpen ? (
                  <div className="record-list" role="list" aria-label="Sequence enrollments">
                    {sequenceEnrollments.length === 0 ? (
                      <article className="record-row" role="listitem">
                        <div>
                          <p>No active sequence enrollments.</p>
                        </div>
                      </article>
                    ) : sequenceEnrollments.map((enrollment) => (
                      <article className="record-row" key={enrollment.id} role="listitem">
                        <div>
                          <p>{enrollment.sequenceName}</p>
                          <p className="field-hint">Step {enrollment.currentStepOrder} · next send {formatSequenceTime(enrollment.nextSendAt)}</p>
                        </div>
                        {canWrite ? (
                          <div>
                            <Button className="button-secondary" type="button" onClick={() => handleCancelSequenceEnrollment(enrollment.id)}>Cancel</Button>
                          </div>
                        ) : null}
                      </article>
                    ))}
                  </div>
                ) : null}
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <h3>Notes</h3>
                {canWrite ? (
                  <form className="auth-form" onSubmit={handleCreateNote}>
                    <Field label="New note">
                      <textarea className="text-input" value={noteBody} onChange={(event) => setNoteBody(event.target.value)} rows={4} />
                    </Field>
                    <Button type="submit">Add note</Button>
                  </form>
                ) : null}
                <div className="record-list" role="list" aria-label="Notes list">
                  {selectedNotes.map((note) => (
                    <article className="record-row" key={note.id} role="listitem">
                      <div>
                        <p>{note.body}</p>
                        <p className="field-hint">{note.createdByUserName || 'Unknown author'}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <div className="section-header">
                  <h3>Tasks</h3>
                  <Button className="button-secondary" type="button" onClick={handleOpenContactTasks}>
                    Open in tasks
                  </Button>
                </div>
                {canWrite ? (
                  <form className="auth-form" onSubmit={handleCreateTask}>
                    <Field label="Task title">
                      <input className="text-input" value={taskForm.title} onChange={(event) => setTaskForm((current) => ({ ...current, title: event.target.value }))} required />
                    </Field>
                    <Field label="Task description">
                      <textarea className="text-input" value={taskForm.description} onChange={(event) => setTaskForm((current) => ({ ...current, description: event.target.value }))} rows={3} />
                    </Field>
                    <Field label="Assigned to">
                      <select className="text-input" value={taskForm.assignedToUserId} onChange={(event) => setTaskForm((current) => ({ ...current, assignedToUserId: event.target.value }))}>
                        {userOptions.map((user) => (
                          <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                        ))}
                      </select>
                    </Field>
                    <Field label="Due at">
                      <input className="text-input" type="datetime-local" value={taskForm.dueAt} onChange={(event) => setTaskForm((current) => ({ ...current, dueAt: event.target.value }))} />
                    </Field>
                    <Button type="submit">Save task</Button>
                  </form>
                ) : null}
                <div className="record-list" role="list" aria-label="Contact tasks list">
                  {selectedTasks.map((task) => (
                    <article className="record-row" key={task.id} role="listitem">
                      <div>
                        <p>{task.title}</p>
                        <p className="field-hint">{task.assignedToUserName || 'Unassigned'}</p>
                      </div>
                    </article>
                  ))}
                </div>
              </div>
            </Card>
            <Card>
              <div className="card-stack">
                <h3>Activity</h3>
                <ActivityTimeline activities={selectedActivities} ariaLabel="Activity list" />
              </div>
            </Card>
          </div>
        </Card>
      ) : null}
    </section>
  )
}

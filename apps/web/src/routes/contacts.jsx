import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { bulkStatusOptions } from '../components/ui/bulk_actions'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { archiveContact, contactsExportURL, createContact, getContact, listContacts, updateContact } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { createNote } from '../lib/notes'
import { evaluateContactLeadScore } from '../lib/lead_scoring'
import { createTask, listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'
import { customFieldFilterFromParams, listCustomFields } from '../lib/custom_fields'
import { usePageTitle } from '../lib/use_page_title'
import { ContactEmailCard, ContactSequencesCard } from './contact_communications'
import { ContactFoundationCommunications } from './contact_foundation_communications'
import { ContactForm } from './contact_form'
import { ContactAttributionCard, ContactLeadScoreCard } from './contact_insights'
import { ClientAccountContext } from './client_account_context'
import { ClientReviewSchedule, refreshClientReviewTasks } from './client_review_schedule'
import { ContactListCard } from './contact_list'
import {
  contactFormValues,
  contactPayload,
  duplicateSearchTerm,
  emptyContactForm,
  emptyContactTaskForm,
  formatContactAddress,
  fullContactName,
  relatedPipelineLabels
} from './contact_view'
import { RecordWorkCards } from './record_work'
import { TouchpointSummary } from './touchpoint_summary'
import { useContactOutreach } from './use_contact_outreach'
const showFoundationCommunications = import.meta.env.DEV

export function ContactsRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { contactId } = useParams()
  const { session, businessProfile, canWrite } = useAuth()
  const routeContactId = Number.parseInt(contactId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
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
  const [foundationCommunicationsSnapshot, setFoundationCommunicationsSnapshot] = useState('')
  const [leadScoreStatus, setLeadScoreStatus] = useState('')
  const [isEvaluatingLeadScore, setIsEvaluatingLeadScore] = useState(false)
  const {
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
  } = useContactOutreach({ selectedContactId, onError: setError })
  const searchControllerRef = useRef(null)

  const selectedContact = detail?.contact || null
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedDeals = detail?.deals || []
  const hasFilter = search.trim() !== '' || ownerFilter !== 'all' || customFilter.fieldKey !== ''
  const selectedActivities = detail?.activities || []

  useLayoutEffect(() => {
    setFoundationCommunicationsSnapshot('')
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

  const handleClientReviewChanged = () => refreshClientReviewTasks('contact', selectedContactId, setDetail, setDetailCache)

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
            <ClientReviewSchedule entityType="contact" entityId={selectedContactId} isClient={selectedContact.isClient || selectedContact.status === 'customer'} canWrite={canWrite} users={userOptions} onChanged={handleClientReviewChanged} />
            {showFoundationCommunications ? (
              <ContactFoundationCommunications
                key={selectedContactId}
                canWrite={canWrite}
                contact={selectedContact}
                contactId={selectedContactId}
                onError={setError}
                onSnapshotChange={setFoundationCommunicationsSnapshot}
              />
            ) : null}
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
            <TouchpointSummary entityType="contact" entityId={selectedContactId} refreshKey={JSON.stringify({ selectedActivities, selectedNotes, selectedTasks, emailHistory, foundationCommunicationsSnapshot })} />
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

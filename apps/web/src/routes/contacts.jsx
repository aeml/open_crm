import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { bulkStatusOptions } from '../components/ui/bulk_actions'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { archiveContact, contactsExportURL, createContact, listContacts, updateContact } from '../lib/contacts'
import { listOrganizationUsers } from '../lib/users'
import { customFieldFilterFromParams, listCustomFields } from '../lib/custom_fields'
import { crmExportOwnership, crmExportSetupURL } from '../lib/crm_exports'
import { usePageTitle } from '../lib/use_page_title'
import { ContactListCard } from './contact_list'
import {
  contactPayload,
  duplicateSearchTerm,
  relatedPipelineLabels
} from './contact_view'
import { ContactCreateWorkspace, ContactWorkspace } from './contact_workspace'
import { useContactDetail } from './use_contact_detail'
import { useContactOutreach } from './use_contact_outreach'
import { requireRecordResponse } from './use_record_selection'

export function ContactsRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { contactId } = useParams()
  const { session, businessProfile, canWrite } = useAuth()
  const routeContactId = Number.parseInt(contactId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const canExport = ['owner', 'admin'].includes(session?.membership?.role || '')
  const pipelineLabels = relatedPipelineLabels(businessType)
  usePageTitle('Contacts')
  const initialSearch = searchParams.get('q') || ''
  const initialOwnerFilter = searchParams.get('owner') || 'all'
  const initialCustomFilter = customFieldFilterFromParams(searchParams)
  const [contacts, setContacts] = useState([])
  const [selectedContactIds, setSelectedContactIds] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState(initialSearch)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [customFilter, setCustomFilter] = useState(initialCustomFilter)
  const [userOptions, setUserOptions] = useState([])
  const [customDefinitions, setCustomDefinitions] = useState([])
  const [customDefinitionsLoaded, setCustomDefinitionsLoaded] = useState(false)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [duplicateCandidate, setDuplicateCandidate] = useState(null)
  const contactDetail = useContactDetail({
    customDefinitions,
    customDefinitionsLoaded,
    navigateToContact: (nextContactId) => navigate(`/contacts/${nextContactId}`),
    routeContactId,
    setError,
    userOptions
  })
  const {
    clear: clearContactDetail,
    detail,
    fillForm: fillFormFromDetail,
    form,
    isArchiving: isArchivingContact,
    isDetailLoading,
    isSaving: isSavingContact,
    mode,
    open: openContactDetail,
    selectedContactId,
    selection: contactSelection,
    setDetail,
    setForm,
    setIsArchiving: setIsArchivingContact,
    setIsSaving: setIsSavingContact,
    startCreate: startContactCreate,
    work: contactWork
  } = contactDetail
  const contactOutreach = useContactOutreach({ selectedContactId, onError: setError })
  const {
    activities: selectedActivities,
    load: loadWork,
    notes: selectedNotes,
    refreshTasks,
    setTaskForm,
    tasks: selectedTasks
  } = contactWork
  const searchControllerRef = useRef(null)

  const selectedContact = detail?.contact || null
  const selectedDeals = detail?.deals || []
  const hasFilter = search.trim() !== '' || ownerFilter !== 'all' || customFilter.fieldKey !== ''

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
    clearContactDetail()
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

  const handleClientReviewChanged = refreshTasks

  function handleOpenContact(contact) {
    if (routeContactId === contact.id) return
    openContactDetail(contact.id)
  }

  async function handleCreate(event) {
    event.preventDefault()
    const operation = contactSelection.start('create', selectedContactId, { allowEmpty: true, group: 'contact-snapshot' })
    if (!operation) return
    setIsSavingContact(true)
    try {
      const data = await createContact(contactPayload(form, customDefinitions))
      if (!data?.contact?.id) throw new Error('Unable to create contact.')
      setContacts((current) => [...current, data.contact])
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      if (contactSelection.isCurrent(operation.selection)) {
        navigate(`/contacts/${data.contact.id}`)
        setError('')
        setDuplicateSearch('')
        setDuplicateCandidate(null)
      }
    } catch (saveError) {
      if (contactSelection.isCurrent(operation.selection)) {
        setError(saveError.message || 'Unable to create contact.')
        setDuplicateSearch(duplicateSearchTerm(saveError.message, form.email || `${form.firstName} ${form.lastName}`))
        setDuplicateCandidate(saveError.duplicate || null)
      }
    } finally {
      contactSelection.finish(operation)
      if (contactSelection.isCurrent(operation.selection)) setIsSavingContact(false)
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    const operation = contactSelection.start('update', selectedContactId, { group: 'contact-snapshot' })
    if (!operation) return
    setIsSavingContact(true)
    try {
      const data = requireRecordResponse(await updateContact(operation.entityId, contactPayload(form, customDefinitions)), 'contact', operation.entityId, 'Unable to update contact.')
      if (!contactSelection.canApply(operation)) return
      setContacts((current) => current.map((entry) => (entry.id === operation.entityId ? data.contact : entry)))
      if (!contactSelection.isCurrent(operation.selection)) return
      const detailData = { ...data, deals: detail?.deals || [] }
      setDetail(detailData)
      fillFormFromDetail(detailData)
      loadWork({ notes: selectedNotes, tasks: selectedTasks, activities: data.activities || selectedActivities })
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      if (contactSelection.isCurrent(operation.selection)) {
        setError(saveError.message || 'Unable to update contact.')
        setDuplicateSearch(duplicateSearchTerm(saveError.message, form.email || `${form.firstName} ${form.lastName}`))
        setDuplicateCandidate(saveError.duplicate || null)
      }
    } finally {
      contactSelection.finish(operation)
      if (contactSelection.isCurrent(operation.selection)) setIsSavingContact(false)
    }
  }

  async function handleArchive() {
    const operation = contactSelection.start('archive', selectedContactId, { group: 'contact-snapshot' })
    if (!operation) return
    setIsArchivingContact(true)
    try {
      await archiveContact(operation.entityId)
      setContacts((current) => current.filter((entry) => entry.id !== operation.entityId))
      setMeta((current) => ({ ...current, total: Math.max(0, current.total - 1) }))
      if (!contactSelection.isEntityActive(operation.entityId)) return
      clearContactDetail()
      navigate('/contacts')
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (archiveError) {
      if (contactSelection.isCurrent(operation.selection)) setError(archiveError.message || 'Unable to archive contact.')
    } finally {
      contactSelection.finish(operation)
      if (contactSelection.isCurrent(operation.selection)) setIsArchivingContact(false)
    }
  }

  return (
    <section className="dashboard-grid contacts-grid">
      <ContactListCard
        bulkActions={{ entityType: 'contact', selectedIds: selectedContactIds, visibleIds: contacts.map((contact) => contact.id), onSelectionChange: setSelectedContactIds, onChanged: () => reloadContacts(search, ownerFilter), statuses: bulkStatusOptions.contact, userOptions }}
        canWrite={canWrite}
        canExport={canExport}
        contacts={contacts}
        currentUserId={currentUserId}
        customDefinitions={customDefinitions}
        customFilter={customFilter}
        duplicateCandidate={duplicateCandidate}
        duplicateSearch={duplicateSearch}
        error={error}
        exportURL={contactsExportURL({ search, customField: customFilter, ...crmExportOwnership(ownerFilter) })}
        durableExportURL={crmExportSetupURL({ resource: 'contacts', search, customField: customFilter, ...crmExportOwnership(ownerFilter) })}
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
          startContactCreate()
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
        <ContactCreateWorkspace
          customDefinitions={customDefinitions}
          form={form}
          isSaving={isSavingContact}
          onSetForm={setForm}
          onSubmit={handleCreate}
        />
      ) : null}

      {mode === 'detail' && selectedContact ? (
        <ContactWorkspace
          canWrite={canWrite}
          contact={selectedContact}
          customDefinitions={customDefinitions}
          deals={selectedDeals}
          form={form}
          isArchiving={isArchivingContact}
          isLoading={isDetailLoading}
          isSaving={isSavingContact}
          onArchive={handleArchive}
          onCreateDeal={handleCreateRelatedDeal}
          onError={setError}
          onOpenDeal={handleOpenDeal}
          onOpenTasks={handleOpenContactTasks}
          onReviewChanged={handleClientReviewChanged}
          onSetForm={setForm}
          onUpdate={handleUpdate}
          outreach={contactOutreach}
          pipelineLabels={pipelineLabels}
          users={userOptions}
          work={contactWork}
        />
      ) : null}
    </section>
  )
}

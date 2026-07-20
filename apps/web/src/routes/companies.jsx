import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { RecordEmailComposer } from '../components/record_email_composer'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { archiveCompany, createCompany, getCompany, listCompanies, sendCompanyEmail, updateCompany } from '../lib/companies'
import { createContact, listContacts } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { createNote, listNotes } from '../lib/notes'
import { createTask, listTasks } from '../lib/tasks'
import { listOrganizationUsers } from '../lib/users'
import { customFieldFilterFromParams, customFieldPayload, listCustomFields } from '../lib/custom_fields'
import { usePageTitle } from '../lib/use_page_title'
import {
  buildClientRecords,
  buildCompanyPayload,
  companyFormValues,
  createDescription,
  detailSubtitle,
  duplicateSearchTerm,
  emailRecipientOptions,
  individualClientFromContact,
  isIndividualClient,
  organizationClientFromCompany,
  primaryLinkedContactID,
  relatedPipelineLabels,
  sortContactOptions,
  splitFullName
} from './company_view'
import { CompanyForm } from './company_form'
import { CompanyDirectory } from './company_directory'
import { CompanyPeople } from './company_people'
import { useCompanyPeople } from './use_company_people'
import { ClientAccountContext } from './client_account_context'
import { ClientReviewSchedule, refreshClientReviewTasks } from './client_review_schedule'
import { ClientHealthReport } from './client_health_report'
import { RecordWorkCards } from './record_work'
import { TouchpointSummary } from './touchpoint_summary'

const emptyForm = {
  name: '',
  clientType: 'organization',
  addressLine1: '',
  addressLine2: '',
  city: '',
  state: '',
  postalCode: '',
  country: '',
  industry: '',
  email: '',
  phone: '',
  website: '',
  status: 'prospect',
  linkedContactIDs: '',
  customFields: {}
}

const emptyTaskForm = {
  title: '',
  description: '',
  dueAt: '',
  assignedToUserId: ''
}


export function CompaniesRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { companyId } = useParams()
  const { session, businessProfile, canWrite } = useAuth()
  const routeCompanyId = Number.parseInt(companyId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const pipelineLabels = relatedPipelineLabels(businessType)
  usePageTitle('Companies')
  const initialSearch = searchParams.get('q') || ''
  const initialOwnerFilter = searchParams.get('owner') || 'all'
  const initialCustomFilter = customFieldFilterFromParams(searchParams)
  const [mode, setMode] = useState('list')
  const [companies, setCompanies] = useState([])
  const [bulkEntityType, setBulkEntityType] = useState('company')
  const [selectedClientIds, setSelectedClientIds] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0 })
  const [search, setSearch] = useState(initialSearch)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [customFilter, setCustomFilter] = useState(initialCustomFilter)
  const [selectedCompanyId, setSelectedCompanyId] = useState(null)
  const [detail, setDetail] = useState(null)
  const [detailCache, setDetailCache] = useState({})
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [ownerOptions, setOwnerOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [companyCustomDefinitions, setCompanyCustomDefinitions] = useState([])
  const [contactCustomDefinitions, setContactCustomDefinitions] = useState([])
  const [customDefinitionsLoaded, setCustomDefinitionsLoaded] = useState(false)
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [duplicateCandidate, setDuplicateCandidate] = useState(null)
  const searchControllerRef = useRef(null)

  const selectedCompany = detail?.company || null
  const linkedContacts = detail?.linkedContacts || []
  const selectedNotes = detail?.notes || []
  const selectedTasks = detail?.tasks || []
  const selectedDeals = detail?.deals || []
  const hasFilter = search.trim() !== '' || ownerFilter !== 'all' || customFilter.fieldKey !== ''
  const selectedActivities = detail?.activities || []
  const companyEmailRecipients = useMemo(() => emailRecipientOptions(linkedContacts), [linkedContacts])
  const companyPeople = useCompanyPeople({
    selectedCompanyId,
    selectedCompany,
    customDefinitions: contactCustomDefinitions,
    onCreated: handleLinkedPersonCreated,
    onError: handleLinkedPersonError
  })

  function buildCompaniesPath(nextSearch = search, nextOwner = ownerFilter, nextCustomFilter = customFilter) {
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
    return `/companies${suffix}`
  }

  async function loadCompanies(nextSearch = '', nextOwner = 'all', { signal, nextCustomFilter = customFilter } = {}) {
    const isUnassigned = nextOwner === 'unassigned'
    const ownerUserId = isUnassigned || nextOwner === 'all' ? 0 : Number.parseInt(nextOwner, 10) || 0
    const [companyData, contactData] = await Promise.all([
      listCompanies({ search: nextSearch, unassigned: isUnassigned, ownerUserId, customField: nextCustomFilter }, { signal }),
      listContacts({ search: nextSearch, unassigned: isUnassigned, ownerUserId }, { signal })
    ])

    if (Array.isArray(companyData?.companies)) {
      const nextCompanies = companyData.companies
      const nextContacts = nextCustomFilter.fieldKey ? [] : (contactData?.contacts || []).filter((contact) => contact.isClient)
      const clients = buildClientRecords(nextCompanies, nextContacts)
      setCompanies(clients)
      setSelectedClientIds([])
        setMeta({ page: 1, pageSize: 20, total: clients.length })
        return
      }

    if (companyData?.company) {
      const entry = organizationClientFromCompany(companyData.company)
      setCompanies([entry])
       setMeta({ page: 1, pageSize: 20, total: 1 })
       setDetailCache((current) => ({ ...current, [entry.id]: companyData }))
       return
     }

     setCompanies([])
     setMeta({ page: 1, pageSize: 20, total: 0 })
  }

  async function loadContactOptions({ signal } = {}) {
    const data = await listContacts('', { signal })
    setContactOptions(sortContactOptions(data.contacts || []))
  }

  async function loadUserOptions({ signal } = {}) {
    const nextOwners = await listOrganizationUsers({ includeDisabled: true, signal })
    const nextUsers = nextOwners.filter((user) => (user.status || 'active') === 'active')
    setOwnerOptions(nextOwners)
    setUserOptions(nextUsers)
    setTaskForm((current) => {
      if (current.assignedToUserId || nextUsers.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(nextUsers[0].id) }
    })
  }

  function fillFormFromDetail(data) {
    setForm(companyFormValues(data.company, data.linkedContacts || [], companyCustomDefinitions))
  }

  async function loadCustomDefinitions({ signal } = {}) {
    const [companyDefinitions, contactDefinitions] = await Promise.all([listCustomFields('company', { signal }), listCustomFields('contact', { signal })])
    setCompanyCustomDefinitions(companyDefinitions)
    setContactCustomDefinitions(contactDefinitions)
    setCustomDefinitionsLoaded(true)
  }

  useEffect(() => {
    const controller = new AbortController()

    async function run() {
      setIsListLoading(true)
      try {
        await Promise.all([loadCompanies(initialSearch, initialOwnerFilter, { signal: controller.signal }), loadContactOptions({ signal: controller.signal }), loadUserOptions({ signal: controller.signal }), loadCustomDefinitions({ signal: controller.signal })])
        setError('')
        setDuplicateSearch('')
        setDuplicateCandidate(null)
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load companies.')
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
    navigate(buildCompaniesPath(value, ownerFilter), { replace: true })
    await reloadCompanies(value, ownerFilter)
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
    setSelectedCompanyId(null)
    navigate(buildCompaniesPath(nextSearch, nextOwner, nextCustomFilter), { replace: true })
    await reloadCompanies(nextSearch, nextOwner, nextCustomFilter)
  }

  async function reloadCompanies(nextSearch = search, nextOwner = ownerFilter, nextCustomFilter = customFilter) {
    searchControllerRef.current?.abort()
    const controller = new AbortController()
    searchControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadCompanies(nextSearch, nextOwner, { signal: controller.signal, nextCustomFilter })
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load companies.')
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
    navigate(buildCompaniesPath(search, value), { replace: true })
    await reloadCompanies(search, value)
  }

  async function applyCustomFilter(nextCustomFilter) {
    setCustomFilter(nextCustomFilter)
    navigate(buildCompaniesPath(search, ownerFilter, nextCustomFilter), { replace: true })
    await reloadCompanies(search, ownerFilter, nextCustomFilter)
  }

  async function handleDuplicateSearch() {
    if (!duplicateSearch) {
      return
    }

    setSearch(duplicateSearch)
    setMode('list')
    setDetail(null)
    setSelectedCompanyId(null)
    navigate('/companies')
    try {
      await loadCompanies(duplicateSearch)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load companies.')
    }
  }

  function handleOpenDuplicate() {
    if (!duplicateCandidate?.id || !duplicateCandidate?.entityType) {
      return
    }
    if (duplicateCandidate.entityType === 'contact') {
      navigate(`/contacts/${duplicateCandidate.id}`)
      return
    }
    navigate(`/companies/${duplicateCandidate.id}`)
  }

  function handleOpenDeal(dealID) {
    navigate(`/deals/${dealID}`)
  }

  function handleCreateRelatedDeal() {
    if (!selectedCompanyId) {
      return
    }
    const params = new URLSearchParams({ companyId: String(selectedCompanyId) })
    const contactID = primaryLinkedContactID(linkedContacts)
    if (contactID > 0) {
      params.set('primaryContactId', String(contactID))
    }
    navigate(`/deals?${params.toString()}`)
  }

  function handleOpenCompanyTasks() {
    if (!selectedCompanyId) {
      return
    }
    navigate(`/tasks?entityType=company&entityId=${selectedCompanyId}`)
  }

  const handleClientReviewChanged = () => refreshClientReviewTasks('company', selectedCompanyId, setDetail, setDetailCache)

  async function handleOpenCompany(company) {
    if (company.entityType === 'contact') {
      navigate(`/contacts/${company.entityId}`)
      return
    }

    const companyID = company.id
    const cached = detailCache[companyID]
    if (cached) {
      setSelectedCompanyId(companyID)
      setDetail(cached)
      fillFormFromDetail(cached)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/companies/${companyID}`)
      return
    }

    try {
        const [data, notes, taskData, dealData] = await Promise.all([
          getCompany(companyID),
          listNotes('company', companyID),
          listTasks({ status: 'open', entityType: 'company', entityId: companyID }),
          listDeals({ companyId: companyID })
        ])
        const detailData = { ...data, notes, tasks: taskData.tasks || [], deals: dealData.deals || [] }
      setDetailCache((current) => ({ ...current, [companyID]: detailData }))
      setSelectedCompanyId(companyID)
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/companies/${companyID}`)
      setError('')
    } catch (loadError) {
      setError(loadError.message || 'Unable to load company.')
    }
  }

  useEffect(() => {
    const controller = new AbortController()

    async function openRouteCompany() {
      if (!customDefinitionsLoaded) return
      if (!Number.isInteger(routeCompanyId) || routeCompanyId <= 0) {
        if (selectedCompanyId || mode === 'detail') {
          setSelectedCompanyId(null)
          setDetail(null)
          setForm(emptyForm)
          setNoteBody('')
          setTaskForm(emptyTaskForm)
          setMode('list')
        }
        return
      }

      if (selectedCompanyId === routeCompanyId && detail?.company?.id === routeCompanyId) {
        return
      }

      const cached = detailCache[routeCompanyId]
      if (cached) {
        setSelectedCompanyId(routeCompanyId)
        setDetail(cached)
        fillFormFromDetail(cached)
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setMode('detail')
        setError('')
        return
      }

      try {
        setIsDetailLoading(true)
        const [data, notes, taskData, dealData] = await Promise.all([
          getCompany(routeCompanyId, { signal: controller.signal }),
          listNotes('company', routeCompanyId, { signal: controller.signal }),
          listTasks({ status: 'open', entityType: 'company', entityId: routeCompanyId }, { signal: controller.signal }),
          listDeals({ companyId: routeCompanyId }, { signal: controller.signal })
        ])
        if (controller.signal.aborted) {
          return
        }
        const detailData = { ...data, notes, tasks: taskData.tasks || [], deals: dealData.deals || [] }
        setDetailCache((current) => ({ ...current, [routeCompanyId]: detailData }))
        setSelectedCompanyId(routeCompanyId)
        setDetail(detailData)
        fillFormFromDetail(detailData)
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setMode('detail')
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load company.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsDetailLoading(false)
        }
      }
    }

    openRouteCompany()
    return () => {
      controller.abort()
    }
  }, [customDefinitionsLoaded, detail, detailCache, mode, routeCompanyId, selectedCompanyId])

  async function handleCreate(event) {
    event.preventDefault()
    try {
      if (isIndividualClient(form.clientType)) {
        const fullName = splitFullName(form.name)
        const data = await createContact({
          firstName: fullName.firstName,
          lastName: fullName.lastName,
          email: form.email,
          phone: form.phone,
          addressLine1: form.addressLine1,
          addressLine2: form.addressLine2,
          city: form.city,
          state: form.state,
          postalCode: form.postalCode,
          country: form.country,
          jobTitle: '',
          status: form.status,
          isClient: true,
          customFields: customFieldPayload(contactCustomDefinitions, form.customFields)
        })
        const nextClient = individualClientFromContact(data.contact)
        setCompanies((current) => [...current, nextClient].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId))
        setMeta((current) => ({ ...current, total: current.total + 1 }))
        setForm(emptyForm)
        setMode('list')
        navigate(`/contacts/${data.contact.id}`)
        setError('')
        setDuplicateSearch('')
        setDuplicateCandidate(null)
        return
      }

      const data = await createCompany({
        ...buildCompanyPayload(form, companyCustomDefinitions)
      })
      if (!data?.company?.id) {
        throw new Error('Unable to create company.')
      }
      const detailData = { ...data, notes: data.notes || [], tasks: data.tasks || [], deals: [] }
      setDetailCache((current) => ({ ...current, [data.company.id]: detailData }))
      setCompanies((current) => [...current, organizationClientFromCompany(data.company)].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId))
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      setSelectedCompanyId(data.company.id)
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('detail')
      navigate(`/companies/${data.company.id}`)
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to create company.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.website || form.email || form.phone || form.name))
      setDuplicateCandidate(saveError.duplicate || null)
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    if (!selectedCompanyId) {
      return
    }

    try {
      const data = await updateCompany(selectedCompanyId, {
        ...buildCompanyPayload(form, companyCustomDefinitions)
      })
      if (!data?.company?.id) {
        throw new Error('Unable to update company.')
      }
      const detailData = { ...data, notes: detail?.notes || [], tasks: detail?.tasks || [], deals: detail?.deals || [] }
      setDetailCache((current) => ({ ...current, [selectedCompanyId]: detailData }))
      setCompanies((current) => current.map((entry) => (entry.id === selectedCompanyId ? organizationClientFromCompany(data.company) : entry)))
      setDetail(detailData)
      fillFormFromDetail(detailData)
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      setError(saveError.message || 'Unable to update company.')
      setDuplicateSearch(duplicateSearchTerm(saveError.message, form.website || form.email || form.phone || form.name))
      setDuplicateCandidate(saveError.duplicate || null)
    }
  }

  function handleLinkedPersonCreated(result) {
    const linkedContact = { ...result.contact, ...result.link }
    setContactOptions((current) => sortContactOptions([...current.filter((entry) => entry.id !== result.contact.id), result.contact]))
    setDetail((current) => {
      if (current?.company?.id !== selectedCompanyId) return current
      const nextActivities = result.activity?.id ? [result.activity, ...(current.activities || []).filter((entry) => entry.id !== result.activity.id)] : (current.activities || [])
      const detailData = { ...current, linkedContacts: [...(current.linkedContacts || []).filter((entry) => entry.id !== linkedContact.id), linkedContact], activities: nextActivities }
      setDetailCache((cache) => ({ ...cache, [selectedCompanyId]: detailData }))
      return detailData
    })
    setError('')
    setDuplicateSearch('')
    setDuplicateCandidate(null)
  }

  function handleLinkedPersonError(saveError, searchTerm) {
    setError(saveError.message || 'Unable to add linked person.')
    setDuplicateSearch(duplicateSearchTerm(saveError.message, searchTerm))
    setDuplicateCandidate(saveError.duplicate || null)
  }

  function handleAddClient() {
    navigate('/companies')
    setMode('create')
    setForm(emptyForm)
    setDetail(null)
    setSelectedCompanyId(null)
  }

  function handleClearFilters() {
    const clearedCustomFilter = { fieldKey: '', operator: '', value: '' }
    setSearch('')
    setOwnerFilter('all')
    setCustomFilter(clearedCustomFilter)
    navigate(buildCompaniesPath('', 'all', clearedCustomFilter), { replace: true })
    reloadCompanies('', 'all', clearedCustomFilter)
  }

  function handleToggleClientSelection(company) {
    if (bulkEntityType !== company.entityType) {
      setBulkEntityType(company.entityType)
      setSelectedClientIds([company.entityId])
      return
    }
    setSelectedClientIds((current) => current.includes(company.entityId) ? current.filter((id) => id !== company.entityId) : [...current, company.entityId])
  }

  async function handleArchive() {
    if (!selectedCompanyId) {
      return
    }

    try {
      await archiveCompany(selectedCompanyId)
      setCompanies((current) => current.filter((entry) => entry.id !== selectedCompanyId))
      setMeta((current) => ({ ...current, total: Math.max(0, current.total - 1) }))
      setDetail((current) => {
        if (!current?.company?.id) {
          return null
        }
        const next = { ...detailCache }
        delete next[current.company.id]
        setDetailCache(next)
        return null
      })
      setSelectedCompanyId(null)
      setForm(emptyForm)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setMode('list')
      navigate('/companies')
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive company.')
    }
  }

  async function handleCreateNote(event) {
    event.preventDefault()
    if (!selectedCompanyId || !noteBody.trim()) {
      return
    }

    try {
      const data = await createNote({
        entityType: 'company',
        entityId: selectedCompanyId,
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
        setDetailCache((cache) => ({ ...cache, [selectedCompanyId]: next }))
        return next
      })
      setNoteBody('')
      setError('')
    } catch (noteError) {
      setError(noteError.message || 'Unable to add note.')
    }
  }

  async function handleCreateTask(event) {
    event.preventDefault()
    if (!selectedCompanyId || !taskForm.title.trim()) {
      return
    }

    try {
      const data = await createTask({
        entityType: 'company',
        entityId: selectedCompanyId,
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
        setDetailCache((cache) => ({ ...cache, [selectedCompanyId]: next }))
        return next
      })
      setTaskForm(emptyTaskForm)
      setError('')
    } catch (taskError) {
      setError(taskError.message || 'Unable to create task.')
    }
  }

  const detailTitle = useMemo(() => selectedCompany?.name || '', [selectedCompany])

  return (
    <section className="dashboard-grid contacts-grid">
      <CompanyDirectory
        bulkEntityType={bulkEntityType}
        canWrite={canWrite}
        companies={companies}
        currentUserId={currentUserId}
        customDefinitions={companyCustomDefinitions}
        customFilter={customFilter}
        duplicateCandidate={duplicateCandidate}
        duplicateSearch={duplicateSearch}
        error={error}
        hasFilter={hasFilter}
        isLoading={isListLoading}
        meta={meta}
        onAddClient={handleAddClient}
        onApplyCustomFilter={applyCustomFilter}
        onApplyOwnerFilter={applyOwnerFilter}
        onApplySavedView={handleApplySavedView}
        onBulkChanged={() => reloadCompanies(search, ownerFilter)}
        onClearFilters={handleClearFilters}
        onDuplicateSearch={handleDuplicateSearch}
        onOpenClient={handleOpenCompany}
        onOpenDuplicate={handleOpenDuplicate}
        onReload={() => reloadCompanies(search)}
        onSearchChange={handleSearchChange}
        onSelectionChange={setSelectedClientIds}
        onToggleSelection={handleToggleClientSelection}
        ownerFilter={ownerFilter}
        ownerOptions={ownerOptions}
        search={search}
        selectedClientIds={selectedClientIds}
        userOptions={userOptions}
      />

      {mode === 'list' ? <ClientHealthReport canManage={canWrite} owners={ownerOptions} onOpen={(record) => navigate(`/${record.entityType === 'contact' ? 'contacts' : 'companies'}/${record.entityId}`)} /> : null}

      {canWrite && mode === 'create' ? (
        <Card>
            <div className="card-stack">
              <div>
                <h2>New client</h2>
                <p>{createDescription(form.clientType)}</p>
              </div>
            <CompanyForm
              contacts={contactOptions}
              customDefinitions={isIndividualClient(form.clientType) ? contactCustomDefinitions : companyCustomDefinitions}
              form={form}
              onSetForm={setForm}
              onSubmit={handleCreate}
              submitLabel="Save client"
            />
          </div>
        </Card>
      ) : null}

      {mode === 'detail' && selectedCompany ? (
        <Card>
          <div className="card-stack">
            {isDetailLoading ? <p className="field-hint">Loading client detail...</p> : null}
            <div className="section-header">
              <div>
                <h2>{detailTitle}</h2>
                <p>{detailSubtitle(selectedCompany, linkedContacts)}</p>
              </div>
              {canWrite ? (
                <Button className="button-danger" onClick={handleArchive}>
                  Archive client
                </Button>
              ) : null}
            </div>
            <CompanyForm
              canSubmit={canWrite}
              contacts={contactOptions}
              customDefinitions={companyCustomDefinitions}
              form={form}
              includeStatus
              onSetForm={setForm}
              onSubmit={handleUpdate}
              submitLabel="Update client"
            />
            <CompanyPeople
              canWrite={canWrite}
              company={selectedCompany}
              contacts={linkedContacts}
              customDefinitions={contactCustomDefinitions}
              form={companyPeople.form}
              isSaving={companyPeople.isSaving}
              onOpenContact={(contactID) => navigate(`/contacts/${contactID}`)}
              onSetForm={companyPeople.setForm}
              onSubmit={companyPeople.handleSubmit}
              onToggleForm={companyPeople.handleToggleForm}
              showForm={companyPeople.showForm}
            />
            <RecordEmailComposer
              entityType="company"
              entityId={selectedCompanyId}
              canWrite={canWrite}
              recipientOptions={companyEmailRecipients}
              sendEmail={sendCompanyEmail}
              emptyMessage="Add a linked person with an email address before sending email from this client."
              mergeFieldHint="Merge fields like {{first_name}}, {{company_name}}, and {{client_status}} are filled in when the email is sent."
            />
            <ClientAccountContext
              canWrite={canWrite}
              contacts={linkedContacts}
              deals={selectedDeals}
              isCustomer={selectedCompany.status === 'customer'}
              labels={pipelineLabels}
              notes={selectedNotes}
              onCreateDeal={handleCreateRelatedDeal}
              onOpenContact={(contactID) => navigate(`/contacts/${contactID}`)}
              onOpenDeal={handleOpenDeal}
              tasks={selectedTasks}
            />
            <ClientReviewSchedule entityType="company" entityId={selectedCompanyId}
              isClient={selectedCompany.status === 'customer'} canWrite={canWrite}
              users={userOptions} onChanged={handleClientReviewChanged} />
            <TouchpointSummary entityType="company" entityId={selectedCompanyId} refreshKey={JSON.stringify({ selectedActivities, selectedNotes, selectedTasks, linkedContacts })} />
            <RecordWorkCards
              activities={selectedActivities}
              activityAria="Activity list"
              canWrite={canWrite}
              entityId={selectedCompanyId}
              entityType="company"
              noteBody={noteBody}
              notes={selectedNotes}
              notesAria="Client notes list"
              onCreateNote={handleCreateNote}
              onCreateTask={handleCreateTask}
              onOpenTasks={handleOpenCompanyTasks}
              onSetNoteBody={setNoteBody}
              onSetTaskForm={setTaskForm}
              taskForm={taskForm}
              tasks={selectedTasks}
              tasksAria="Client tasks list"
              users={userOptions}
            />
          </div>
        </Card>
      ) : null}
    </section>
  )
}

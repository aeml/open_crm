import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAuth } from '../app/providers'
import { isAbortError } from '../lib/api'
import { archiveCompany, createCompany, getCompany, listCompanies, updateCompany } from '../lib/companies'
import { createContact, listContacts } from '../lib/contacts'
import { listDeals } from '../lib/deals'
import { listOrganizationUsers } from '../lib/users'
import { customFieldFilterFromParams, customFieldPayload, listCustomFields } from '../lib/custom_fields'
import { usePageTitle } from '../lib/use_page_title'
import {
  buildClientRecords,
  buildCompanyPayload,
  companyFormValues,
  duplicateSearchTerm,
  individualClientFromContact,
  isIndividualClient,
  organizationClientFromCompany,
  primaryLinkedContactID,
  relatedPipelineLabels,
  sortContactOptions,
  splitFullName
} from './company_view'
import { CompanyDirectory } from './company_directory'
import { CompanyCreateWorkspace, CompanyWorkspace } from './company_workspace'
import { useCompanyPeople } from './use_company_people'
import { ClientHealthReport } from './client_health_report'
import { requireRecordResponse, useRecordSelection } from './use_record_selection'
import { useRecordWork } from './use_record_work'

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
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [ownerOptions, setOwnerOptions] = useState([])
  const [form, setForm] = useState(emptyForm)
  const [companyCustomDefinitions, setCompanyCustomDefinitions] = useState([])
  const [contactCustomDefinitions, setContactCustomDefinitions] = useState([])
  const [customDefinitionsLoaded, setCustomDefinitionsLoaded] = useState(false)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [isSavingCompany, setIsSavingCompany] = useState(false)
  const [isArchivingCompany, setIsArchivingCompany] = useState(false)
  const [duplicateSearch, setDuplicateSearch] = useState('')
  const [duplicateCandidate, setDuplicateCandidate] = useState(null)
  const searchControllerRef = useRef(null)
  const seededRouteCompanyRef = useRef(null)
  const companySelection = useRecordSelection(selectedCompanyId)
  const companyWork = useRecordWork({
    defaultAssignedToUserId: userOptions[0]?.id ? String(userOptions[0].id) : '',
    entityType: 'company',
    selectedEntityId: selectedCompanyId,
    selection: companySelection,
    onError: setError
  })
  const {
    activities: selectedActivities,
    fetchWork,
    load: loadWork,
    notes: selectedNotes,
    refreshTasks,
    reset: resetWork,
    setActivities,
    setTaskForm,
    tasks: selectedTasks
  } = companyWork

  const selectedCompany = detail?.company || null
  const linkedContacts = detail?.linkedContacts || []
  const selectedDeals = detail?.deals || []
  const hasFilter = search.trim() !== '' || ownerFilter !== 'all' || customFilter.fieldKey !== ''
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
    companySelection.clear()
    resetWork()
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
    companySelection.clear()
    resetWork()
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

  const handleClientReviewChanged = refreshTasks

  async function handleOpenCompany(company) {
    if (company.entityType === 'contact') {
      navigate(`/contacts/${company.entityId}`)
      return
    }

    const companyID = company.entityId
    if (routeCompanyId === companyID) return
    companySelection.begin(companyID)
    setSelectedCompanyId(companyID)
    setDetail(null)
    setForm(emptyForm)
    resetWork()
    setMode('detail')
    setIsDetailLoading(true)
    navigate(`/companies/${companyID}`)
  }

  useEffect(() => {
    async function openRouteCompany() {
      if (!customDefinitionsLoaded) return
      if (!Number.isInteger(routeCompanyId) || routeCompanyId <= 0) {
        seededRouteCompanyRef.current = null
        companySelection.clear()
        if (selectedCompanyId || mode === 'detail') {
          setSelectedCompanyId(null)
          setDetail(null)
          setForm(emptyForm)
          resetWork()
          setMode('list')
        }
        setIsDetailLoading(false)
        setIsSavingCompany(false)
        setIsArchivingCompany(false)
        return
      }

      const seededRouteCompany = seededRouteCompanyRef.current
      if (seededRouteCompany?.companyId === routeCompanyId && companySelection.isCurrent(seededRouteCompany.selection)) {
        setIsDetailLoading(false)
        return
      }
      seededRouteCompanyRef.current = null
      const activeSelection = companySelection.begin(routeCompanyId)
      const signal = activeSelection.controller.signal
      setSelectedCompanyId(routeCompanyId)
      setDetail(null)
      setForm(emptyForm)
      resetWork()
      setMode('detail')
      setIsSavingCompany(false)
      setIsArchivingCompany(false)

      try {
        setIsDetailLoading(true)
        const [data, work, dealData] = await Promise.all([
          getCompany(routeCompanyId, { signal }),
          fetchWork(routeCompanyId, { signal }),
          listDeals({ companyId: routeCompanyId }, { signal })
        ])
        if (!companySelection.isCurrent(activeSelection)) return
        requireRecordResponse(data, 'company', routeCompanyId, 'Unable to load company.')
        const detailData = { ...data, deals: dealData.deals || [] }
        setDetail(detailData)
        fillFormFromDetail(detailData)
        loadWork({ ...work, activities: data.activities || [] })
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError) && companySelection.isCurrent(activeSelection)) {
          setError(loadError.message || 'Unable to load company.')
        }
      } finally {
        if (companySelection.isCurrent(activeSelection)) setIsDetailLoading(false)
      }
    }

    openRouteCompany()
  }, [customDefinitionsLoaded, routeCompanyId])

  async function handleCreate(event) {
    event.preventDefault()
    const operation = companySelection.start('create', selectedCompanyId, { allowEmpty: true, group: 'company-snapshot' })
    if (!operation) return
    setIsSavingCompany(true)
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
        if (!data?.contact?.id) throw new Error('Unable to create individual client.')
        const nextClient = individualClientFromContact(data.contact)
        setCompanies((current) => [...current, nextClient].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId))
        setMeta((current) => ({ ...current, total: current.total + 1 }))
        if (companySelection.isCurrent(operation.selection)) {
          navigate(`/contacts/${data.contact.id}`)
          setError('')
          setDuplicateSearch('')
          setDuplicateCandidate(null)
        }
        return
      }

      const data = await createCompany({
        ...buildCompanyPayload(form, companyCustomDefinitions)
      })
      if (!data?.company?.id) {
        throw new Error('Unable to create company.')
      }
      setCompanies((current) => [...current, organizationClientFromCompany(data.company)].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId))
      setMeta((current) => ({ ...current, total: current.total + 1 }))
      if (companySelection.isCurrent(operation.selection)) {
        const companyID = data.company.id
        const activeSelection = companySelection.begin(companyID)
        const detailData = { ...data, deals: data.deals || [] }
        seededRouteCompanyRef.current = { companyId: companyID, selection: activeSelection }
        setSelectedCompanyId(companyID)
        setDetail(detailData)
        fillFormFromDetail(detailData)
        loadWork({ notes: data.notes || [], tasks: data.tasks || [], activities: data.activities || [] })
        setMode('detail')
        setIsDetailLoading(false)
        setIsSavingCompany(false)
        setIsArchivingCompany(false)
        navigate(`/companies/${data.company.id}`)
        setError('')
        setDuplicateSearch('')
        setDuplicateCandidate(null)
      }
    } catch (saveError) {
      if (companySelection.isCurrent(operation.selection)) {
        setError(saveError.message || 'Unable to create company.')
        setDuplicateSearch(duplicateSearchTerm(saveError.message, form.website || form.email || form.phone || form.name))
        setDuplicateCandidate(saveError.duplicate || null)
      }
    } finally {
      companySelection.finish(operation)
      if (companySelection.isCurrent(operation.selection)) setIsSavingCompany(false)
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    const operation = companySelection.start('update', selectedCompanyId, { group: 'company-snapshot' })
    if (!operation) return
    setIsSavingCompany(true)
    try {
      const data = requireRecordResponse(await updateCompany(operation.entityId, {
        ...buildCompanyPayload(form, companyCustomDefinitions)
      }), 'company', operation.entityId, 'Unable to update company.')
      if (!companySelection.canApply(operation)) return
      setCompanies((current) => current.map((entry) => (entry.entityType === 'company' && entry.entityId === operation.entityId ? organizationClientFromCompany(data.company) : entry)))
      if (!companySelection.isCurrent(operation.selection)) return
      const detailData = { ...data, deals: detail?.deals || [] }
      setDetail(detailData)
      fillFormFromDetail(detailData)
      loadWork({ notes: selectedNotes, tasks: selectedTasks, activities: data.activities || selectedActivities })
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (saveError) {
      if (companySelection.isCurrent(operation.selection)) {
        setError(saveError.message || 'Unable to update company.')
        setDuplicateSearch(duplicateSearchTerm(saveError.message, form.website || form.email || form.phone || form.name))
        setDuplicateCandidate(saveError.duplicate || null)
      }
    } finally {
      companySelection.finish(operation)
      if (companySelection.isCurrent(operation.selection)) setIsSavingCompany(false)
    }
  }

  function handleLinkedPersonCreated(result) {
    const linkedContact = { ...result.contact, ...result.link }
    setContactOptions((current) => sortContactOptions([...current.filter((entry) => entry.id !== result.contact.id), result.contact]))
    setDetail((current) => {
      if (current?.company?.id !== selectedCompanyId) return current
      return { ...current, linkedContacts: [...(current.linkedContacts || []).filter((entry) => entry.id !== linkedContact.id), linkedContact] }
    })
    if (result.activity?.id) setActivities((current) => [result.activity, ...current.filter((entry) => entry.id !== result.activity.id)])
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
    companySelection.clear()
    navigate('/companies')
    setMode('create')
    setForm(emptyForm)
    setDetail(null)
    setSelectedCompanyId(null)
    resetWork()
    setIsSavingCompany(false)
    setIsArchivingCompany(false)
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
    const operation = companySelection.start('archive', selectedCompanyId, { group: 'company-snapshot' })
    if (!operation) return
    setIsArchivingCompany(true)
    try {
      await archiveCompany(operation.entityId)
      setCompanies((current) => current.filter((entry) => entry.entityType !== 'company' || entry.entityId !== operation.entityId))
      setMeta((current) => ({ ...current, total: Math.max(0, current.total - 1) }))
      if (!companySelection.isEntityActive(operation.entityId)) return
      setIsArchivingCompany(false)
      companySelection.clear()
      setDetail(null)
      setSelectedCompanyId(null)
      setForm(emptyForm)
      resetWork()
      setMode('list')
      navigate('/companies')
      setError('')
      setDuplicateSearch('')
      setDuplicateCandidate(null)
    } catch (archiveError) {
      if (companySelection.isCurrent(operation.selection)) setError(archiveError.message || 'Unable to archive company.')
    } finally {
      companySelection.finish(operation)
      if (companySelection.isCurrent(operation.selection)) setIsArchivingCompany(false)
    }
  }

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
        <CompanyCreateWorkspace
          companyCustomDefinitions={companyCustomDefinitions}
          contactCustomDefinitions={contactCustomDefinitions}
          contacts={contactOptions}
          form={form}
          isSaving={isSavingCompany}
          onSetForm={setForm}
          onSubmit={handleCreate}
        />
      ) : null}

      {mode === 'detail' && selectedCompany ? (
        <CompanyWorkspace
          canWrite={canWrite}
          company={selectedCompany}
          companyPeople={companyPeople}
          companyCustomDefinitions={companyCustomDefinitions}
          contactCustomDefinitions={contactCustomDefinitions}
          contactOptions={contactOptions}
          form={form}
          isArchiving={isArchivingCompany}
          isLoading={isDetailLoading}
          isSaving={isSavingCompany}
          linkedContacts={linkedContacts}
          onArchive={handleArchive}
          onCreateDeal={handleCreateRelatedDeal}
          onOpenContact={(contactID) => navigate(`/contacts/${contactID}`)}
          onOpenDeal={handleOpenDeal}
          onOpenTasks={handleOpenCompanyTasks}
          onReviewChanged={handleClientReviewChanged}
          onSetForm={setForm}
          onUpdate={handleUpdate}
          pipelineLabels={pipelineLabels}
          selectedDeals={selectedDeals}
          users={userOptions}
          work={companyWork}
        />
      ) : null}
    </section>
  )
}

import { useEffect } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { useAuth } from '../app/providers'
import { archiveCompany, createCompany, updateCompany } from '../lib/companies'
import { createContact } from '../lib/contacts'
import { customFieldFilterFromParams, customFieldPayload } from '../lib/custom_fields'
import { usePageTitle } from '../lib/use_page_title'
import {
  buildCompanyPayload,
  duplicateSearchTerm,
  individualClientFromContact,
  isIndividualClient,
  organizationClientFromCompany,
  relatedPipelineLabels,
  splitFullName
} from './company_view'
import { CompanyDirectory } from './company_directory'
import { CompanyCreateWorkspace, CompanyWorkspace } from './company_workspace'
import { buildCompaniesPath, useCompanyDirectory } from './use_company_directory'
import { useCompanyDetail } from './use_company_detail'
import { useCompanyPeople } from './use_company_people'
import { useCompanyLinkedContacts } from './use_company_linked_contacts'
import { useContactLookup } from './use_contact_lookup'
import { ClientHealthReport } from './client_health_report'
import { requireRecordResponse } from './use_record_selection'

export function CompaniesRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { companyId } = useParams()
  const { session, businessProfile, canWrite } = useAuth()
  const routeCompanyId = Number.parseInt(companyId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const canExport = ['owner', 'admin'].includes(session?.membership?.role || '')
  const pipelineLabels = relatedPipelineLabels(businessType)
  usePageTitle('Companies')
  const initialSearch = searchParams.get('q') || ''
  const initialOwnerFilter = searchParams.get('owner') || 'all'
  const initialCustomFilter = customFieldFilterFromParams(searchParams)
  const directory = useCompanyDirectory({ initialCustomFilter, initialOwnerFilter, initialSearch })
  const {
    bulkEntityType,
    companies,
    companyCustomDefinitions,
    contactCustomDefinitions,
    customDefinitionsLoaded,
    customFilter,
    duplicateCandidate,
    duplicateSearch,
    error,
    isListLoading,
    meta,
    ownerFilter,
    ownerOptions,
    reloadCompanies,
    search,
    selectedClientIds,
    setBulkEntityType,
    setCompanies,
    setCustomFilter,
    setDuplicateCandidate,
    setDuplicateSearch,
    setError,
    setMeta,
    setOwnerFilter,
    setSearch,
    setSelectedClientIds,
    userOptions
  } = directory
  const contactLookup = useContactLookup()
  const companyDetail = useCompanyDetail({
    companyCustomDefinitions,
    customDefinitionsLoaded,
    navigateToCompany: (nextCompanyId) => navigate(`/companies/${nextCompanyId}`),
    routeCompanyId,
    setError,
    userOptions
  })
  const {
    clear: clearCompanyDetail,
    detail,
    fillForm: fillFormFromDetail,
    form,
    isArchiving: isArchivingCompany,
    isDetailLoading,
    isSaving: isSavingCompany,
    mode,
    open: openCompanyDetail,
    seedCreated: seedCreatedCompany,
    selectedCompanyId,
    selection: companySelection,
    setDetail,
    setForm,
    setIsArchiving: setIsArchivingCompany,
    setIsSaving: setIsSavingCompany,
    startCreate: startCompanyCreate,
    work: companyWork
  } = companyDetail
  const {
    activities: selectedActivities,
    load: loadWork,
    notes: selectedNotes,
    refreshTasks,
    setActivities,
    setTaskForm,
    tasks: selectedTasks
  } = companyWork

  const selectedCompany = detail?.company || null
  const companyLinks = useCompanyLinkedContacts({
    companyId: selectedCompanyId,
    initialContacts: detail?.linkedContacts,
    initialMeta: detail?.linkedContactMeta
  })
  const linkedContacts = companyLinks.contacts
  const selectedDeals = detail?.deals || []
  const hasFilter = search.trim() !== '' || ownerFilter !== 'all' || customFilter.fieldKey !== ''
  const companyPeople = useCompanyPeople({
    selectedCompanyId,
    selectedCompany,
    customDefinitions: contactCustomDefinitions,
    onCreated: handleLinkedPersonCreated,
    onError: handleLinkedPersonError,
    onRelationshipsChanged: companyLinks.refresh
  })

  useEffect(() => {
    setTaskForm((current) => {
      if (current.assignedToUserId || userOptions.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(userOptions[0].id) }
    })
  }, [userOptions])

  async function handleSearchChange(event) {
    const value = event.target.value
    setSearch(value)
    navigate(buildCompaniesPath(value, ownerFilter, customFilter), { replace: true })
    await reloadCompanies(value, ownerFilter, customFilter)
  }

  async function handleApplySavedView(filters) {
    const nextSearch = filters.q || ''
    const nextOwner = filters.owner || 'all'
    const nextCustomFilter = { fieldKey: filters.customField || '', operator: filters.customOperator || '', value: filters.customValue || '' }
    setSearch(nextSearch)
    setOwnerFilter(nextOwner)
    setCustomFilter(nextCustomFilter)
    clearCompanyDetail()
    navigate(buildCompaniesPath(nextSearch, nextOwner, nextCustomFilter), { replace: true })
    await reloadCompanies(nextSearch, nextOwner, nextCustomFilter)
  }

  async function applyOwnerFilter(value) {
    setOwnerFilter(value)
    navigate(buildCompaniesPath(search, value, customFilter), { replace: true })
    await reloadCompanies(search, value, customFilter)
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
    setOwnerFilter('all')
    setCustomFilter({ fieldKey: '', operator: '', value: '' })
    clearCompanyDetail()
    navigate('/companies')
    await reloadCompanies(duplicateSearch, 'all', { fieldKey: '', operator: '', value: '' })
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
    const contactID = companyLinks.primaryContact?.id || 0
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
    openCompanyDetail(companyID)
  }

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
        seedCreatedCompany(data)
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
        ...buildCompanyPayload(form, companyCustomDefinitions, { includeLinkedContacts: false })
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
    contactLookup.includeContacts([result.contact])
    companyLinks.include(linkedContact)
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
    navigate('/companies')
    startCompanyCreate()
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
      clearCompanyDetail()
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
        canExport={canExport}
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
        onBulkChanged={() => reloadCompanies(search, ownerFilter, customFilter)}
        onClearFilters={handleClearFilters}
        onDuplicateSearch={handleDuplicateSearch}
        onOpenClient={handleOpenCompany}
        onOpenDuplicate={handleOpenDuplicate}
        onReload={() => reloadCompanies(search, ownerFilter, customFilter)}
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
          contactLookup={contactLookup}
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
          contactLookup={contactLookup}
          companyCustomDefinitions={companyCustomDefinitions}
          contactCustomDefinitions={contactCustomDefinitions}
          form={form}
          isArchiving={isArchivingCompany}
          isLoading={isDetailLoading}
          isSaving={isSavingCompany}
          linkedContacts={linkedContacts}
          linkedContactDirectory={companyLinks}
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

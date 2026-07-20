import { useEffect, useMemo } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { archiveDeal, createDeal, updateDeal, updateDealStage } from '../lib/deals'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import {
  dealFormValues,
  pipelineLabels,
  stagesForPipeline
} from './deal_view'
import { emptyCloseReview, stageOutcome } from './deal_close_review'
import { DealDirectory } from './deal_directory'
import { DealCreateCard } from './deal_editor'
import { DealWorkspace } from './deal_workspace'
import { useDealDirectory } from './use_deal_directory'
import { useDealDetail } from './use_deal_detail'
import { requireDealResponse } from './use_deal_selection'

const emptyForm = {
  name: '',
  stageId: '',
  companyId: '',
  primaryContactId: '',
  valueAmount: '',
  valueCurrency: 'USD',
  expectedCloseDate: '',
  ownerUserId: '',
  ...emptyCloseReview
}

export function DealsRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { dealId } = useParams()
  const { session, businessProfile, canWrite } = useAuth()
  const routeDealId = Number.parseInt(dealId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const labels = pipelineLabels(businessType)
  usePageTitle(labels.collection)
  const initialSearch = searchParams.get('q') || ''
  const initialPipelineFilter = searchParams.get('pipeline') || 'all'
  const initialStageFilter = searchParams.get('stage') || 'all'
  const initialOwnerFilter = searchParams.get('owner') || 'all'
  const initialCloseFrom = searchParams.get('closeFrom') || ''
  const initialCloseTo = searchParams.get('closeTo') || ''
  const initialCompanyId = searchParams.get('companyId') || ''
  const initialPrimaryContactId = searchParams.get('primaryContactId') || ''
  const initialForm = {
    ...emptyForm,
    companyId: initialCompanyId,
    primaryContactId: initialPrimaryContactId
  }
  const directory = useDealDirectory({
    initialCloseFrom,
    initialCloseTo,
    initialCompanyId,
    initialForm,
    initialOwnerFilter,
    initialPipelineFilter,
    initialPrimaryContactId,
    initialSearch,
    initialStageFilter,
    routeDealId
  })
  const {
    buildDealsPath,
    closeFrom,
    closeTo,
    companyOptions,
    contactOptions,
    deals,
    error,
    form,
    isListLoading,
    meta,
    ownerFilter,
    pipelines,
    pipelineFilter,
    pipelineReady,
    reloadDeals,
    search,
    selectedDealIds,
    setCloseFrom,
    setCloseTo,
    setDeals,
    setError,
    setForm,
    setMeta,
    setOwnerFilter,
    setPipelineFilter,
    setSearch,
    setSelectedDealIds,
    setStageFilter,
    stages,
    stageFilter,
    userOptions
  } = directory
  const dealDetail = useDealDetail({
    deals,
    navigateToDeal: (nextDealId) => navigate(buildDealsPath(nextDealId)),
    pipelineReady,
    routeDealId,
    setDeals,
    setError,
    userOptions
  })
  const {
    clear: clearDealDetail,
    commercial: dealCommercial,
    detailForm,
    isDetailLoading,
    select: handleSelectDeal,
    selectedDeal,
    selectedDealId,
    selectedStageId,
    selection: dealSelection,
    setDetailForm,
    setSelectedDealId,
    setSelectedStageId,
    setStageCloseReview,
    stageCloseReview,
    work: dealWork
  } = dealDetail
  const {
    fetchWork,
    load: loadWork,
    setActivities,
    setTaskForm
  } = dealWork
  const {
    load: loadCommercials,
    refresh: refreshCommercials
  } = dealCommercial
  const filteredStages = stagesForPipeline(stages, pipelineFilter)
  const hasDealFilters = search.trim() !== '' || pipelineFilter !== 'all' || stageFilter !== 'all' || ownerFilter !== 'all' || closeFrom !== '' || closeTo !== ''

  useEffect(() => {
    if (filteredStages.length > 0 && !selectedStageId) {
      setSelectedStageId(String(filteredStages[0].id))
    }
  }, [filteredStages, selectedStageId])

  useEffect(() => {
    setTaskForm((current) => {
      if (current.assignedToUserId || userOptions.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(userOptions[0].id) }
    })
  }, [userOptions])

  const moveStage = stages.find((stage) => String(stage.id) === String(selectedStageId))
  const dealEmailRecipients = useMemo(() => {
    if (!selectedDeal?.primaryContactId) {
      return []
    }
    const contact = contactOptions.find((entry) => entry.id === selectedDeal.primaryContactId)
    if (!contact?.email) {
      return []
    }
    const name = `${contact.firstName || ''} ${contact.lastName || ''}`.trim() || selectedDeal.primaryContactName || contact.email
    return [{ id: selectedDeal.primaryContactId, label: `${name} (${contact.email})` }]
  }, [contactOptions, selectedDeal])

  async function handleSearchChange(event) {
    const value = event.target.value
    setSearch(value)
    navigate(buildDealsPath(selectedDealId, value, pipelineFilter, stageFilter, ownerFilter), { replace: true })
    await reloadDeals(value, pipelineFilter, stageFilter, ownerFilter)
  }

  async function handlePipelineFilterChange(event) {
    const value = event.target.value
    const nextStages = stagesForPipeline(stages, value)
    const nextStageFilter = 'all'
    setPipelineFilter(value)
    setStageFilter(nextStageFilter)
    setForm((current) => ({ ...current, stageId: nextStages[0] ? String(nextStages[0].id) : '' }))
    navigate(buildDealsPath(selectedDealId, search, value, nextStageFilter, ownerFilter), { replace: true })
    await reloadDeals(search, value, nextStageFilter, ownerFilter)
  }

  async function handleStageFilterChange(event) {
    const value = event.target.value
    setStageFilter(value)
    navigate(buildDealsPath(selectedDealId, search, pipelineFilter, value, ownerFilter), { replace: true })
    await reloadDeals(search, pipelineFilter, value, ownerFilter)
  }

  async function handleCreate(event) {
    event.preventDefault()
    const operation = dealSelection.start('create', selectedDealId, { allowEmpty: true })
    if (!operation) return
    try {
      const data = await createDeal({
        name: form.name,
        stageId: Number.parseInt(form.stageId, 10),
        companyId: Number.parseInt(form.companyId, 10) || 0,
        primaryContactId: Number.parseInt(form.primaryContactId, 10) || 0,
        valueAmount: form.valueAmount,
        valueCurrency: form.valueCurrency,
        expectedCloseDate: form.expectedCloseDate,
        ownerUserId: Number.parseInt(form.ownerUserId, 10) || 0,
        closeReasonCode: form.closeReasonCode,
        closeNotes: form.closeNotes
      })
      requireDealResponse(data, data?.deal?.id, 'Unable to create deal.')
      setDeals((current) => [...current.filter((entry) => entry.id !== data.deal.id), data.deal])
      setMeta((current) => ({
        ...current,
        total: current.total + 1,
        openCount: current.openCount + (data.deal.status === 'open' ? 1 : 0),
        wonCount: current.wonCount + (data.deal.status === 'won' ? 1 : 0)
      }))
      setForm((current) => ({ ...emptyForm, stageId: current.stageId || form.stageId || (filteredStages[0] ? String(filteredStages[0].id) : stages[0] ? String(stages[0].id) : '') }))
      if (dealSelection.isCurrent(operation.selection)) {
        const activeSelection = dealSelection.begin(data.deal.id)
        setSelectedDealId(data.deal.id)
        setSelectedStageId(String(data.deal.stageId))
        setStageCloseReview(emptyCloseReview)
        setDetailForm(dealFormValues(data.deal))
        loadWork({ activities: data.activities || [] })
        loadCommercials(data)
        navigate(buildDealsPath(data.deal.id))
        setError('')
        try {
          const work = await fetchWork(data.deal.id, { signal: activeSelection.controller.signal })
          if (dealSelection.isCurrent(activeSelection)) loadWork({ ...work, activities: data.activities || [] })
        } catch (workError) {
          if (!isAbortError(workError) && dealSelection.isCurrent(activeSelection)) {
            setError(workError.message || 'Deal created, but its work items could not be loaded.')
          }
        }
      }
    } catch (saveError) {
      if (dealSelection.isCurrent(operation.selection)) setError(saveError.message || 'Unable to create deal.')
    } finally {
      dealSelection.finish(operation)
    }
  }

  async function handleMoveStage() {
    if (!selectedDealId || !selectedStageId) {
      return
    }

    const nextOutcome = stageOutcome(moveStage)
    if ((nextOutcome === 'won' || nextOutcome === 'lost') && !stageCloseReview.closeReasonCode) {
      setError(`Choose a ${nextOutcome} reason before closing this deal.`)
      return
    }

    const operation = dealSelection.start('stage', selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    const previousStatus = selectedDeal?.status || 'open'
    try {
      const data = requireDealResponse(await updateDealStage(operation.dealId, {
        stageId: Number.parseInt(selectedStageId, 10),
        closeReasonCode: stageCloseReview.closeReasonCode,
        closeNotes: stageCloseReview.closeNotes
      }), operation.dealId, 'Unable to move deal.')
      if (!dealSelection.canApply(operation)) return
      setDeals((current) => current.map((entry) => (entry.id === operation.dealId ? data.deal : entry)))
      setMeta((current) => ({
        ...current,
        openCount: current.openCount + (data.deal.status === 'open' ? 1 : 0) - (previousStatus === 'open' ? 1 : 0),
        wonCount: current.wonCount + (data.deal.status === 'won' ? 1 : 0) - (previousStatus === 'won' ? 1 : 0)
      }))
      if (!dealSelection.isCurrent(operation.selection)) return
      setSelectedStageId(String(data.deal.stageId))
      setStageCloseReview(emptyCloseReview)
      setDetailForm(dealFormValues(data.deal))
      setActivities(data.activities || [])
      refreshCommercials(data)
      setError('')
    } catch (moveError) {
      if (dealSelection.isCurrent(operation.selection)) setError(moveError.message || 'Unable to move deal.')
    } finally {
      dealSelection.finish(operation)
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    const operation = dealSelection.start('update', selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    try {
      const data = requireDealResponse(await updateDeal(operation.dealId, {
        name: detailForm.name,
        companyId: Number.parseInt(detailForm.companyId, 10) || 0,
        primaryContactId: Number.parseInt(detailForm.primaryContactId, 10) || 0,
        valueAmount: detailForm.valueAmount,
        valueCurrency: detailForm.valueCurrency,
        expectedCloseDate: detailForm.expectedCloseDate,
        ownerUserId: Number.parseInt(detailForm.ownerUserId, 10) || 0
      }), operation.dealId)
      if (!dealSelection.canApply(operation)) return
      setDeals((current) => current.map((entry) => (entry.id === operation.dealId ? data.deal : entry)))
      if (!dealSelection.isCurrent(operation.selection)) return
      setDetailForm(dealFormValues(data.deal))
      setActivities(data.activities || [])
      refreshCommercials(data)
      setError('')
    } catch (saveError) {
      if (dealSelection.isCurrent(operation.selection)) setError(saveError.message || 'Unable to update deal.')
    } finally {
      dealSelection.finish(operation)
    }
  }

  async function handleArchive() {
    const operation = dealSelection.start('archive', selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    const archivedStatus = selectedDeal?.status || 'open'
    try {
      await archiveDeal(operation.dealId)
      setDeals((current) => current.filter((entry) => entry.id !== operation.dealId))
      setMeta((current) => ({
        ...current,
        total: Math.max(0, current.total - 1),
        openCount: Math.max(0, current.openCount - (archivedStatus === 'open' ? 1 : 0)),
        wonCount: Math.max(0, current.wonCount - (archivedStatus === 'won' ? 1 : 0))
      }))
      if (!dealSelection.isDealActive(operation.dealId)) return
      clearDealDetail()
      navigate(buildDealsPath(null))
      setError('')
    } catch (archiveError) {
      if (dealSelection.isCurrent(operation.selection)) setError(archiveError.message || 'Unable to archive deal.')
    } finally {
      dealSelection.finish(operation)
    }
  }

  async function applyOwnerFilter(value) {
    setOwnerFilter(value)
    navigate(buildDealsPath(selectedDealId, search, pipelineFilter, stageFilter, value), { replace: true })
    await reloadDeals(search, pipelineFilter, stageFilter, value)
  }

  async function handleCloseDateFilter(event) {
    event.preventDefault()
    navigate(buildDealsPath(selectedDealId, search, pipelineFilter, stageFilter, ownerFilter, closeFrom, closeTo), { replace: true })
    await reloadDeals(search, pipelineFilter, stageFilter, ownerFilter, closeFrom, closeTo)
  }

  async function handleApplySavedView(filters) {
    const nextSearch = filters.q || ''
    const nextPipelineFilter = filters.pipeline || 'all'
    const nextStageFilter = filters.stage || 'all'
    const nextOwnerFilter = filters.owner || 'all'
    const nextCloseFrom = filters.closeFrom || ''
    const nextCloseTo = filters.closeTo || ''
    const nextStages = stagesForPipeline(stages, nextPipelineFilter)
    setSearch(nextSearch)
    setPipelineFilter(nextPipelineFilter)
    setStageFilter(nextStageFilter)
    setOwnerFilter(nextOwnerFilter)
    setCloseFrom(nextCloseFrom)
    setCloseTo(nextCloseTo)
    setForm((current) => ({ ...current, stageId: nextStages[0] ? String(nextStages[0].id) : current.stageId }))
    clearDealDetail()
    navigate(buildDealsPath(null, nextSearch, nextPipelineFilter, nextStageFilter, nextOwnerFilter, nextCloseFrom, nextCloseTo), { replace: true })
    await reloadDeals(nextSearch, nextPipelineFilter, nextStageFilter, nextOwnerFilter, nextCloseFrom, nextCloseTo)
  }

  function handleClearFilters() {
    setSearch('')
    setPipelineFilter('all')
    setStageFilter('all')
    setOwnerFilter('all')
    setCloseFrom('')
    setCloseTo('')
    navigate(buildDealsPath(null, '', 'all', 'all', 'all', '', ''), { replace: true })
    reloadDeals('', 'all', 'all', 'all', '', '')
  }

  function handleToggleSelection(dealID) {
    setSelectedDealIds((current) => current.includes(dealID) ? current.filter((id) => id !== dealID) : [...current, dealID])
  }

  function handleOpenDealTasks() {
    if (!selectedDealId) {
      return
    }
    navigate(`/tasks?entityType=deal&entityId=${selectedDealId}`)
  }

  return (
    <section className="dashboard-grid contacts-grid">
      <DealDirectory
        canWrite={canWrite}
        closeFrom={closeFrom}
        closeTo={closeTo}
        currentUserId={currentUserId}
        deals={deals}
        error={error}
        filteredStages={filteredStages}
        hasFilters={hasDealFilters}
        isLoading={isListLoading}
        labels={labels}
        meta={meta}
        onApplyCloseDates={handleCloseDateFilter}
        onApplyOwnerFilter={applyOwnerFilter}
        onApplySavedView={handleApplySavedView}
        onBulkChanged={() => reloadDeals(search, pipelineFilter, stageFilter, ownerFilter)}
        onClearFilters={handleClearFilters}
        onCloseFromChange={setCloseFrom}
        onCloseToChange={setCloseTo}
        onOpenDeal={handleSelectDeal}
        onPipelineChange={handlePipelineFilterChange}
        onReload={() => reloadDeals(search, pipelineFilter, stageFilter, ownerFilter)}
        onSearchChange={handleSearchChange}
        onSelectionChange={setSelectedDealIds}
        onStageChange={handleStageFilterChange}
        onToggleSelection={handleToggleSelection}
        ownerFilter={ownerFilter}
        pipelines={pipelines}
        pipelineFilter={pipelineFilter}
        search={search}
        selectedDealIds={selectedDealIds}
        stageFilter={stageFilter}
        users={userOptions}
      />

      {canWrite ? (
        <DealCreateCard
          companies={companyOptions}
          contacts={contactOptions}
          form={form}
          labels={labels}
          onSetForm={setForm}
          onSubmit={handleCreate}
          pipelineFilter={pipelineFilter}
          stages={filteredStages}
          users={userOptions}
        />
      ) : null}

      {selectedDeal ? (
        <DealWorkspace
          canWrite={canWrite}
          commercial={dealCommercial}
          companies={companyOptions}
          contacts={contactOptions}
          deal={selectedDeal}
          detail={{ form: detailForm, isLoading: isDetailLoading, onArchive: handleArchive, onSetForm: setDetailForm, onSubmit: handleUpdate }}
          emailRecipients={dealEmailRecipients}
          labels={labels}
          onOpenTasks={handleOpenDealTasks}
          stage={{ onMove: handleMoveStage, onSetReview: setStageCloseReview, onSetStage: setSelectedStageId, review: stageCloseReview, selectedStageId, stages }}
          users={userOptions}
          work={dealWork}
        />
      ) : null}
    </section>
  )
}

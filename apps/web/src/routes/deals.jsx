import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { RecordEmailComposer } from '../components/record_email_composer'
import { archiveDeal, createDeal, getDeal, listDeals, listDealPipelines, sendDealEmail, updateDeal, updateDealStage } from '../lib/deals'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listProductCatalogItems } from '../lib/product_catalog'
import { listOrganizationUsers } from '../lib/users'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'
import {
  dealFormValues,
  emptyDealMeta,
  flattenPipelineStages,
  pipelineLabels,
  stagesForPipeline
} from './deal_view'
import { DealLineItemsCard, DealSignatureCard } from './deal_quote'
import { emptyCloseReview, stageOutcome } from './deal_close_review'
import { DealDirectory } from './deal_directory'
import { DealCreateCard, DealDetailsEditor, DealStageMover } from './deal_editor'
import { RecordWorkCards } from './record_work'
import { useDealCommercials } from './use_deal_commercials'
import { requireDealResponse, useDealSelection } from './use_deal_selection'
import { useDealWork } from './use_deal_work'

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
  const [pipelines, setPipelines] = useState([])
  const [stages, setStages] = useState([])
  const [deals, setDeals] = useState([])
  const [selectedDealIds, setSelectedDealIds] = useState([])
  const [meta, setMeta] = useState(emptyDealMeta)
  const [form, setForm] = useState({
    ...emptyForm,
    companyId: initialCompanyId,
    primaryContactId: initialPrimaryContactId
  })
  const [detailForm, setDetailForm] = useState(emptyForm)
  const [search, setSearch] = useState(initialSearch)
  const [pipelineFilter, setPipelineFilter] = useState(initialPipelineFilter)
  const [stageFilter, setStageFilter] = useState(initialStageFilter)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [closeFrom, setCloseFrom] = useState(initialCloseFrom)
  const [closeTo, setCloseTo] = useState(initialCloseTo)
  const [companyOptions, setCompanyOptions] = useState([])
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [selectedDealId, setSelectedDealId] = useState(null)
  const [selectedStageId, setSelectedStageId] = useState('')
  const [stageCloseReview, setStageCloseReview] = useState(emptyCloseReview)
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [pipelineReady, setPipelineReady] = useState(false)
  const dealSelection = useDealSelection(selectedDealId)
  const {
    activities,
    fetchWork,
    handleCreateNote,
    handleCreateTask,
    isCreatingNote,
    isCreatingTask,
    load: loadWork,
    noteBody,
    notes,
    reset: resetWork,
    setActivities,
    setNoteBody,
    setTaskForm,
    taskForm,
    tasks
  } = useDealWork({
    defaultAssignedToUserId: userOptions[0]?.id ? String(userOptions[0].id) : '',
    selectedDealId,
    selection: dealSelection,
    onError: setError
  })
  const {
    handleAddLineItem,
    handleCatalogLineItemChange,
    handleCreateSignatureRequest,
    handleRemoveLineItem,
    handleSaveLineItems,
    handleUpdateSignatureRequestStatus,
    isCreatingSignatureRequest,
    isSavingLineItems,
    lineItemForm,
    lineItems,
    lineTotals,
    load: loadCommercials,
    productCatalogItems,
    refresh: refreshCommercials,
    reset: resetCommercials,
    setLineItemForm,
    setSignatureForm,
    signatureForm,
    signatureRequests,
    updatingSignatureRequestId
  } = useDealCommercials({ selectedDealId, selection: dealSelection, onDealUpdated: applyCommercialDealUpdate, onError: setError })
  const listControllerRef = useRef(null)
  const filteredStages = stagesForPipeline(stages, pipelineFilter)
  const hasDealFilters = search.trim() !== '' || pipelineFilter !== 'all' || stageFilter !== 'all' || ownerFilter !== 'all' || closeFrom !== '' || closeTo !== ''

  function buildDealsPath(nextDealId = routeDealId, nextSearch = search, nextPipelineFilter = pipelineFilter, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, nextCloseFrom = closeFrom, nextCloseTo = closeTo) {
    const params = new URLSearchParams()
    if (nextSearch) {
      params.set('q', nextSearch)
    }
    if (nextPipelineFilter !== 'all') {
      params.set('pipeline', nextPipelineFilter)
    }
    if (nextStageFilter !== 'all') {
      params.set('stage', nextStageFilter)
    }
    if (nextOwnerFilter !== 'all') {
      params.set('owner', nextOwnerFilter)
    }
    if (nextCloseFrom) params.set('closeFrom', nextCloseFrom)
    if (nextCloseTo) params.set('closeTo', nextCloseTo)
    const suffix = params.toString() ? `?${params.toString()}` : ''
    const pathname = nextDealId ? `/deals/${nextDealId}` : '/deals'
    return `${pathname}${suffix}`
  }

  async function loadDeals(nextSearch = search, nextPipelineFilter = pipelineFilter, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, { signal, nextCloseFrom = closeFrom, nextCloseTo = closeTo } = {}) {
    const isUnassigned = nextOwnerFilter === 'unassigned'
    const loadedDeals = await listDeals({
      search: nextSearch,
      pipelineId: nextPipelineFilter === 'all' ? 0 : Number.parseInt(nextPipelineFilter, 10) || 0,
      stageId: nextStageFilter === 'all' ? 0 : Number.parseInt(nextStageFilter, 10) || 0,
      unassigned: isUnassigned,
      ownerUserId: isUnassigned || nextOwnerFilter === 'all' ? 0 : Number.parseInt(nextOwnerFilter, 10) || 0,
      closeFrom: nextCloseFrom,
      closeTo: nextCloseTo
    }, { signal })
    setDeals(loadedDeals.deals || [])
    setSelectedDealIds([])
    setMeta(loadedDeals.meta || emptyDealMeta)
  }

  async function loadPipeline(nextSearch = search, nextPipelineFilter = pipelineFilter, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, { signal, nextCloseFrom = closeFrom, nextCloseTo = closeTo } = {}) {
    const isUnassigned = nextOwnerFilter === 'unassigned'
    const [loadedPipelines, loadedDeals, loadedCompanies, loadedContacts, loadedUsers] = await Promise.all([
      listDealPipelines({ signal }),
      listDeals({
        search: nextSearch,
        pipelineId: nextPipelineFilter === 'all' ? 0 : Number.parseInt(nextPipelineFilter, 10) || 0,
        stageId: nextStageFilter === 'all' ? 0 : Number.parseInt(nextStageFilter, 10) || 0,
        unassigned: isUnassigned,
        ownerUserId: isUnassigned || nextOwnerFilter === 'all' ? 0 : Number.parseInt(nextOwnerFilter, 10) || 0,
        closeFrom: nextCloseFrom,
        closeTo: nextCloseTo
      }, { signal }),
      listCompanies('', { signal }),
      listContacts('', { signal }),
      listOrganizationUsers({ signal })
    ])
    const loadedStages = flattenPipelineStages(loadedPipelines)
    setStages(loadedStages)
    setPipelines(loadedPipelines)
    setDeals(loadedDeals.deals || [])
    setSelectedDealIds([])
    setCompanyOptions(loadedCompanies.companies || [])
    setContactOptions(loadedContacts.contacts || [])
    setUserOptions(loadedUsers)
    setMeta(loadedDeals.meta || emptyDealMeta)
    const nextStages = stagesForPipeline(loadedStages, nextPipelineFilter)
    if (nextStages.length > 0 && !selectedStageId) {
      setSelectedStageId(String(nextStages[0].id))
    }
    setForm((current) => ({
      ...current,
      stageId: current.stageId || (nextStages[0] ? String(nextStages[0].id) : ''),
      companyId: current.companyId || initialCompanyId || (loadedCompanies.companies?.[0] ? String(loadedCompanies.companies[0].id) : ''),
      primaryContactId: current.primaryContactId || initialPrimaryContactId || (loadedContacts.contacts?.[0] ? String(loadedContacts.contacts[0].id) : ''),
      ownerUserId: current.ownerUserId || (loadedUsers[0] ? String(loadedUsers[0].id) : '')
    }))
    setTaskForm((current) => {
      if (current.assignedToUserId || loadedUsers.length === 0) {
        return current
      }
      return { ...current, assignedToUserId: String(loadedUsers[0].id) }
    })
    setPipelineReady(true)
  }

  useEffect(() => {
    const controller = new AbortController()

    async function run() {
      setIsListLoading(true)
      try {
        await loadPipeline(initialSearch, initialPipelineFilter, initialStageFilter, initialOwnerFilter, { signal: controller.signal, nextCloseFrom: initialCloseFrom, nextCloseTo: initialCloseTo })
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setPipelineReady(true)
          setError(loadError.message || 'Unable to load deals.')
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
  }, [initialCloseFrom, initialCloseTo, initialCompanyId, initialOwnerFilter, initialPipelineFilter, initialPrimaryContactId, initialSearch, initialStageFilter])

  const selectedDeal = useMemo(() => deals.find((entry) => entry.id === selectedDealId) || null, [deals, selectedDealId])
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

  useEffect(() => {
    async function syncRouteDeal() {
      if (!pipelineReady) {
        return
      }

      if (!Number.isInteger(routeDealId) || routeDealId <= 0) {
        if (selectedDealId) {
          dealSelection.clear()
          setSelectedDealId(null)
          setSelectedStageId('')
          setStageCloseReview(emptyCloseReview)
          setDetailForm(emptyForm)
          resetWork()
          resetCommercials()
          setIsDetailLoading(false)
        }
        return
      }

      if (selectedDealId === routeDealId) {
        return
      }

      const routeDeal = deals.find((entry) => entry.id === routeDealId)
      if (routeDeal) {
        await handleSelectDeal(routeDeal)
        return
      }

      const activeSelection = dealSelection.begin(routeDealId)
      const signal = activeSelection.controller.signal
      setSelectedDealId(routeDealId)
      setSelectedStageId('')
      setStageCloseReview(emptyCloseReview)
      setDetailForm(emptyForm)
      resetWork()
      resetCommercials()
      try {
        setIsDetailLoading(true)
        const [dealData, work, loadedCatalog] = await Promise.all([
          getDeal(routeDealId, { signal }),
          fetchWork(routeDealId, { signal }),
          listProductCatalogItems({ signal })
        ])
        if (!dealSelection.isCurrent(activeSelection)) return
        requireDealResponse(dealData, routeDealId, 'Unable to load deal.')
        setDeals((current) => {
          const next = current.filter((entry) => entry.id !== routeDealId)
          return [dealData.deal, ...next]
        })
        setSelectedDealId(dealData.deal.id)
        setSelectedStageId(String(dealData.deal.stageId))
        setDetailForm(dealFormValues(dealData.deal))
        loadWork({ ...work, activities: dealData.activities || [] })
        loadCommercials(dealData, loadedCatalog)
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError) && dealSelection.isCurrent(activeSelection)) {
          setError(loadError.message || 'Unable to load deal.')
        }
      } finally {
        if (dealSelection.isCurrent(activeSelection)) setIsDetailLoading(false)
      }
    }

    syncRouteDeal()
  }, [deals, pipelineReady, routeDealId, selectedDealId])

  async function reloadDeals(nextSearch = search, nextPipelineFilter = pipelineFilter, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, nextCloseFrom = closeFrom, nextCloseTo = closeTo) {
    listControllerRef.current?.abort()
    const controller = new AbortController()
    listControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadDeals(nextSearch, nextPipelineFilter, nextStageFilter, nextOwnerFilter, { signal: controller.signal, nextCloseFrom, nextCloseTo })
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setError(loadError.message || 'Unable to load deals.')
      }
    } finally {
      if (listControllerRef.current === controller) {
        listControllerRef.current = null
      }
      if (!controller.signal.aborted) {
        setIsListLoading(false)
      }
    }
  }

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

  async function handleSelectDeal(deal) {
    const activeSelection = dealSelection.begin(deal.id)
    const signal = activeSelection.controller.signal
    setSelectedDealId(deal.id)
    setSelectedStageId(String(deal.stageId))
    setStageCloseReview(emptyCloseReview)
    setDetailForm(dealFormValues(deal))
    resetWork()
    resetCommercials()
    setIsDetailLoading(true)
    navigate(buildDealsPath(deal.id))
    try {
      const [dealData, work, loadedCatalog] = await Promise.all([
        getDeal(deal.id, { signal }),
        fetchWork(deal.id, { signal }),
        listProductCatalogItems({ signal })
      ])
      if (!dealSelection.isCurrent(activeSelection)) return
      requireDealResponse(dealData, deal.id, 'Unable to load deal.')
      setDeals((current) => current.map((entry) => (entry.id === deal.id ? dealData.deal : entry)))
      setDetailForm(dealFormValues(dealData.deal))
      loadWork({ ...work, activities: dealData.activities || [] })
      loadCommercials(dealData, loadedCatalog)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError) && dealSelection.isCurrent(activeSelection)) {
        resetWork()
        setError(loadError.message || 'Unable to load deal.')
      }
    } finally {
      if (dealSelection.isCurrent(activeSelection)) setIsDetailLoading(false)
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
      dealSelection.clear()
      setSelectedDealId(null)
      setSelectedStageId('')
      setStageCloseReview(emptyCloseReview)
      setDetailForm(emptyForm)
      resetWork()
      resetCommercials()
      navigate(buildDealsPath(null))
      setError('')
    } catch (archiveError) {
      if (dealSelection.isCurrent(operation.selection)) setError(archiveError.message || 'Unable to archive deal.')
    } finally {
      dealSelection.finish(operation)
    }
  }

  function applyCommercialDealUpdate(data, dealId, isCurrent) {
    setDeals((current) => current.map((entry) => (entry.id === dealId ? data.deal : entry)))
    if (!isCurrent) return
    setDetailForm(dealFormValues(data.deal))
    setActivities(data.activities || [])
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
    dealSelection.clear()
    setSelectedDealId(null)
    setSelectedStageId('')
    setStageCloseReview(emptyCloseReview)
    setDetailForm(emptyForm)
    resetWork()
    resetCommercials()
    setIsDetailLoading(false)
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
        <Card>
          <div className="card-stack">
            <DealDetailsEditor
              canWrite={canWrite}
              companies={companyOptions}
              contacts={contactOptions}
              deal={selectedDeal}
              form={detailForm}
              isLoading={isDetailLoading}
              labels={labels}
              onArchive={handleArchive}
              onSetForm={setDetailForm}
              onSubmit={handleUpdate}
              users={userOptions}
            />
            <DealLineItemsCard
              canWrite={canWrite}
              deal={selectedDeal}
              form={lineItemForm}
              isSaving={isSavingLineItems}
              items={lineItems}
              labels={labels}
              onAdd={handleAddLineItem}
              onCatalogChange={handleCatalogLineItemChange}
              onRemove={handleRemoveLineItem}
              onSave={handleSaveLineItems}
              onSetForm={setLineItemForm}
              products={productCatalogItems}
              totals={lineTotals}
            />
            <DealSignatureCard
              canWrite={canWrite}
              form={signatureForm}
              isCreating={isCreatingSignatureRequest}
              onCreate={handleCreateSignatureRequest}
              onSetForm={setSignatureForm}
              onUpdate={handleUpdateSignatureRequestStatus}
              requests={signatureRequests}
              updatingID={updatingSignatureRequestId}
            />
            <DealStageMover
              canWrite={canWrite}
              labels={labels}
              onMove={handleMoveStage}
              onSetReview={setStageCloseReview}
              onSetStage={setSelectedStageId}
              review={stageCloseReview}
              selectedStageId={selectedStageId}
              stages={stages}
            />
            <RecordEmailComposer
              entityType="deal"
              entityId={selectedDealId}
              canWrite={canWrite}
              recipientOptions={dealEmailRecipients}
              sendEmail={sendDealEmail}
              emptyMessage="Set a primary contact with an email address before sending email from this deal."
              mergeFieldHint="Merge fields like {{first_name}}, {{deal_name}}, {{deal_stage}}, and {{company_name}} are filled in when the email is sent."
            />
            <RecordWorkCards
              activities={activities}
              activityAria={labels.activityAria}
              canWrite={canWrite}
              entityId={selectedDealId}
              entityType="deal"
              isCreatingNote={isCreatingNote}
              isCreatingTask={isCreatingTask}
              noteBody={noteBody}
              notes={notes}
              notesAria={labels.notesAria}
              onCreateNote={handleCreateNote}
              onCreateTask={handleCreateTask}
              onOpenTasks={handleOpenDealTasks}
              onSetNoteBody={setNoteBody}
              onSetTaskForm={setTaskForm}
              taskForm={taskForm}
              tasks={tasks}
              tasksAria={labels.tasksAria}
              users={userOptions}
            />
          </div>
        </Card>
      ) : null}
    </section>
  )
}

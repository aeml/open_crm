import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { EmptyState } from '../components/ui/empty_state'
import { SavedViews } from '../components/ui/saved_views'
import { InlineError } from '../components/ui/inline_error'
import { BulkActions } from '../components/ui/bulk_actions'
import { RecordEmailComposer } from '../components/record_email_composer'
import { archiveDeal, createDeal, createDealSignatureRequest, dealsExportURL, getDeal, listDeals, listDealPipelines, quotePDFURL, replaceDealLineItems, sendDealEmail, updateDeal, updateDealSignatureRequestStatus, updateDealStage } from '../lib/deals'
import { createNote, listNotes } from '../lib/notes'
import { createTask, listTasks } from '../lib/tasks'
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
  emptyDealsDescription,
  emptyDealsMessage,
  emptyLineItemForm,
  emptyLineTotals,
  emptySignatureForm,
  flattenPipelineStages,
  formatMoney,
  lineItemFormFromCatalogItem,
  lineItemPayload,
  pipelineLabels,
  stageLabel,
  stagesForPipeline
} from './deal_view'
import { DealLineItemsCard, DealSignatureCard } from './deal_quote'
import { CloseReviewFields, DealCloseSummary, emptyCloseReview, stageOutcome } from './deal_close_review'
import { RecordWorkCards } from './record_work'

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

const emptyTaskForm = {
  title: '',
  description: '',
  dueAt: '',
  assignedToUserId: ''
}


export function DealsRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { dealId } = useParams()
  const { session, businessProfile } = useAuth()
  const routeDealId = Number.parseInt(dealId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const canWrite = ['owner', 'admin', 'member'].includes(session?.membership?.role)
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
  const [notes, setNotes] = useState([])
  const [tasks, setTasks] = useState([])
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm)
  const [productCatalogItems, setProductCatalogItems] = useState([])
  const [lineItems, setLineItems] = useState([])
  const [lineItemForm, setLineItemForm] = useState(emptyLineItemForm)
  const [lineTotals, setLineTotals] = useState(emptyLineTotals)
  const [signatureRequests, setSignatureRequests] = useState([])
  const [signatureForm, setSignatureForm] = useState(emptySignatureForm)
  const [activities, setActivities] = useState([])
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [isSavingLineItems, setIsSavingLineItems] = useState(false)
  const [isCreatingSignatureRequest, setIsCreatingSignatureRequest] = useState(false)
  const [updatingSignatureRequestId, setUpdatingSignatureRequestId] = useState(null)
  const [pipelineReady, setPipelineReady] = useState(false)
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
  const createStage = stages.find((stage) => String(stage.id) === String(form.stageId))
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
    const controller = new AbortController()

    async function syncRouteDeal() {
      if (!pipelineReady) {
        return
      }

      if (!Number.isInteger(routeDealId) || routeDealId <= 0) {
        if (selectedDealId) {
          setSelectedDealId(null)
          setSelectedStageId('')
          setStageCloseReview(emptyCloseReview)
          setNotes([])
          setTasks([])
          setActivities([])
          setLineItems([])
          setLineTotals(emptyLineTotals)
          setLineItemForm(emptyLineItemForm)
          setSignatureRequests([])
          setSignatureForm(emptySignatureForm)
          setNoteBody('')
          setTaskForm(emptyTaskForm)
        }
        return
      }

      if (selectedDealId === routeDealId) {
        return
      }

      const routeDeal = deals.find((entry) => entry.id === routeDealId)
      if (routeDeal) {
        setIsDetailLoading(true)
        try {
          await handleSelectDeal(routeDeal, { signal: controller.signal })
        } finally {
          if (!controller.signal.aborted) {
            setIsDetailLoading(false)
          }
        }
        return
      }

      try {
        setIsDetailLoading(true)
        const [dealData, loadedNotes, taskData, loadedCatalog] = await Promise.all([
          getDeal(routeDealId, { signal: controller.signal }),
          listNotes('deal', routeDealId, { signal: controller.signal }),
          listTasks({ status: 'open', entityType: 'deal', entityId: routeDealId }, { signal: controller.signal }),
          listProductCatalogItems({ signal: controller.signal })
        ])
        if (controller.signal.aborted) {
          return
        }
        setDeals((current) => {
          const next = current.filter((entry) => entry.id !== routeDealId)
          return [dealData.deal, ...next]
        })
        setSelectedDealId(dealData.deal.id)
        setSelectedStageId(String(dealData.deal.stageId))
        setDetailForm(dealFormValues(dealData.deal))
        setActivities(dealData.activities || [])
        setLineItems(dealData.lineItems || [])
        setLineTotals(dealData.totals || emptyLineTotals)
        setLineItemForm(emptyLineItemForm)
        setSignatureRequests(dealData.signatureRequests || [])
        setSignatureForm(emptySignatureForm)
        setProductCatalogItems(loadedCatalog)
        setNotes(loadedNotes)
        setTasks(taskData.tasks || [])
        setNoteBody('')
        setTaskForm(emptyTaskForm)
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError)) {
          setError(loadError.message || 'Unable to load deal.')
        }
      } finally {
        if (!controller.signal.aborted) {
          setIsDetailLoading(false)
        }
      }
    }

    syncRouteDeal()
    return () => {
      controller.abort()
    }
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
      const taskData = await listTasks({ status: 'open', entityType: 'deal', entityId: data.deal.id })
      setDeals((current) => [...current, data.deal])
      setNotes(data.notes || [])
      setTasks(taskData.tasks || [])
      setLineItems(data.lineItems || [])
      setLineTotals(data.totals || emptyLineTotals)
      setLineItemForm(emptyLineItemForm)
      setSignatureRequests(data.signatureRequests || [])
      setSignatureForm(emptySignatureForm)
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setActivities(data.activities || [])
      setSelectedDealId(data.deal.id)
      setSelectedStageId(String(data.deal.stageId))
      setDetailForm(dealFormValues(data.deal))
      setMeta((current) => ({
        ...current,
        total: current.total + 1,
        openCount: current.openCount + (data.deal.status === 'open' ? 1 : 0),
        wonCount: current.wonCount + (data.deal.status === 'won' ? 1 : 0)
      }))
      setForm((current) => ({ ...emptyForm, stageId: current.stageId || form.stageId || (filteredStages[0] ? String(filteredStages[0].id) : stages[0] ? String(stages[0].id) : '') }))
      navigate(buildDealsPath(data.deal.id))
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to create deal.')
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

    try {
      const previousStatus = selectedDeal?.status || 'open'
      const data = await updateDealStage(selectedDealId, {
        stageId: Number.parseInt(selectedStageId, 10),
        closeReasonCode: stageCloseReview.closeReasonCode,
        closeNotes: stageCloseReview.closeNotes
      })
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setMeta((current) => ({
        ...current,
        openCount: current.openCount + (data.deal.status === 'open' ? 1 : 0) - (previousStatus === 'open' ? 1 : 0),
        wonCount: current.wonCount + (data.deal.status === 'won' ? 1 : 0) - (previousStatus === 'won' ? 1 : 0)
      }))
      setSelectedStageId(String(data.deal.stageId))
      setStageCloseReview(emptyCloseReview)
      setDetailForm(dealFormValues(data.deal))
      setActivities(data.activities || [])
      setLineItems(data.lineItems || [])
      setLineTotals(data.totals || emptyLineTotals)
      setSignatureRequests(data.signatureRequests || [])
      setError('')
    } catch (moveError) {
      setError(moveError.message || 'Unable to move deal.')
    }
  }

  async function handleSelectDeal(deal, { signal } = {}) {
    setSelectedDealId(deal.id)
    setSelectedStageId(String(deal.stageId))
    setStageCloseReview(emptyCloseReview)
    setDetailForm(dealFormValues(deal))
    setActivities([])
    setLineItems([])
    setLineTotals(emptyLineTotals)
    setLineItemForm(emptyLineItemForm)
    setSignatureRequests([])
    setSignatureForm(emptySignatureForm)
    setNoteBody('')
    setTaskForm(emptyTaskForm)
    navigate(buildDealsPath(deal.id))
    try {
      const [dealData, loadedNotes, taskData, loadedCatalog] = await Promise.all([
        getDeal(deal.id, { signal }),
        listNotes('deal', deal.id, { signal }),
        listTasks({ status: 'open', entityType: 'deal', entityId: deal.id }, { signal }),
        listProductCatalogItems({ signal })
      ])
      if (signal?.aborted) {
        return
      }
      setDeals((current) => current.map((entry) => (entry.id === deal.id ? dealData.deal : entry)))
      setDetailForm(dealFormValues(dealData.deal))
      setActivities(dealData.activities || [])
      setLineItems(dealData.lineItems || [])
      setLineTotals(dealData.totals || emptyLineTotals)
      setSignatureRequests(dealData.signatureRequests || [])
      setProductCatalogItems(loadedCatalog)
      setNotes(loadedNotes)
      setTasks(taskData.tasks || [])
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError)) {
        setNotes([])
        setTasks([])
        setError(loadError.message || 'Unable to load notes.')
      }
    }
  }

  async function handleUpdate(event) {
    event.preventDefault()
    if (!selectedDealId) {
      return
    }

    try {
      const data = await updateDeal(selectedDealId, {
        name: detailForm.name,
        companyId: Number.parseInt(detailForm.companyId, 10) || 0,
        primaryContactId: Number.parseInt(detailForm.primaryContactId, 10) || 0,
        valueAmount: detailForm.valueAmount,
        valueCurrency: detailForm.valueCurrency,
        expectedCloseDate: detailForm.expectedCloseDate,
        ownerUserId: Number.parseInt(detailForm.ownerUserId, 10) || 0
      })
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setDetailForm(dealFormValues(data.deal))
      setActivities(data.activities || [])
      setLineItems(data.lineItems || [])
      setLineTotals(data.totals || emptyLineTotals)
      setSignatureRequests(data.signatureRequests || [])
      setError('')
    } catch (saveError) {
      setError(saveError.message || 'Unable to update deal.')
    }
  }

  async function handleArchive() {
    if (!selectedDealId) {
      return
    }

    try {
      await archiveDeal(selectedDealId)
      setDeals((current) => current.filter((entry) => entry.id !== selectedDealId))
      setMeta((current) => ({
        ...current,
        total: Math.max(0, current.total - 1),
        openCount: Math.max(0, current.openCount - 1)
      }))
      setSelectedDealId(null)
      setSelectedStageId('')
      setStageCloseReview(emptyCloseReview)
      setDetailForm(emptyForm)
      setNotes([])
      setTasks([])
      setLineItems([])
      setLineTotals(emptyLineTotals)
      setLineItemForm(emptyLineItemForm)
      setSignatureRequests([])
      setSignatureForm(emptySignatureForm)
      setActivities([])
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      navigate(buildDealsPath(null))
      setError('')
    } catch (archiveError) {
      setError(archiveError.message || 'Unable to archive deal.')
    }
  }

  async function handleCreateNote(event) {
    event.preventDefault()
    if (!selectedDealId || !noteBody.trim()) {
      return
    }

    try {
      const data = await createNote({
        entityType: 'deal',
        entityId: selectedDealId,
        body: noteBody.trim()
      })
      setNotes((current) => [data.note, ...current])
      setActivities((current) => [data.activity, ...current])
      setNoteBody('')
      setError('')
    } catch (noteError) {
      setError(noteError.message || 'Unable to add note.')
    }
  }

  async function handleCreateTask(event) {
    event.preventDefault()
    if (!selectedDealId || !taskForm.title.trim()) {
      return
    }

    try {
      const data = await createTask({
        entityType: 'deal',
        entityId: selectedDealId,
        title: taskForm.title.trim(),
        description: taskForm.description.trim(),
        status: 'open',
        dueAt: taskForm.dueAt ? `${taskForm.dueAt}:00Z` : '',
        assignedToUserId: Number.parseInt(taskForm.assignedToUserId, 10) || 0
      })
      setTasks((current) => [data.task, ...current.filter((task) => task.id !== data.task.id)])
      setActivities((current) => [...(data.activities || []), ...current])
      setTaskForm(emptyTaskForm)
      setError('')
    } catch (taskError) {
      setError(taskError.message || 'Unable to create task.')
    }
  }

  function handleCatalogLineItemChange(event) {
    const productCatalogItemId = event.target.value
    const catalogItem = productCatalogItems.find((item) => String(item.id) === productCatalogItemId)
    setLineItemForm(lineItemFormFromCatalogItem(catalogItem))
  }

  function handleAddLineItem(event) {
    event.preventDefault()
    if (!lineItemForm.name.trim()) {
      setError('Line item name is required.')
      return
    }
    setLineItems((current) => [...current, { ...lineItemForm, position: current.length + 1 }])
    setLineItemForm(emptyLineItemForm)
    setError('')
  }

  function handleRemoveLineItem(index) {
    setLineItems((current) => current.filter((_, entryIndex) => entryIndex !== index).map((item, entryIndex) => ({ ...item, position: entryIndex + 1 })))
  }

  async function handleSaveLineItems() {
    if (!selectedDealId) {
      return
    }
    setIsSavingLineItems(true)
    try {
      const data = await replaceDealLineItems(selectedDealId, { items: lineItems.map(lineItemPayload) })
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setDetailForm(dealFormValues(data.deal))
      setLineItems(data.lineItems || [])
      setLineTotals(data.totals || emptyLineTotals)
      setSignatureRequests(data.signatureRequests || [])
      setActivities(data.activities || [])
      setError('')
    } catch (lineItemError) {
      setError(lineItemError.message || 'Unable to update deal line items.')
    } finally {
      setIsSavingLineItems(false)
    }
  }

  async function handleCreateSignatureRequest(event) {
    event.preventDefault()
    if (!selectedDealId || !signatureForm.signerName.trim() || !signatureForm.signerEmail.trim()) {
      return
    }
    setIsCreatingSignatureRequest(true)
    try {
      const data = await createDealSignatureRequest(selectedDealId, {
        signerName: signatureForm.signerName.trim(),
        signerEmail: signatureForm.signerEmail.trim()
      })
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setDetailForm(dealFormValues(data.deal))
      setLineItems(data.lineItems || [])
      setLineTotals(data.totals || emptyLineTotals)
      setSignatureRequests(data.signatureRequests || [])
      setSignatureForm(emptySignatureForm)
      setActivities(data.activities || [])
      setError('')
    } catch (signatureError) {
      setError(signatureError.message || 'Unable to create proposal tracking.')
    } finally {
      setIsCreatingSignatureRequest(false)
    }
  }

  async function handleUpdateSignatureRequestStatus(requestID, status) {
    if (!selectedDealId) {
      return
    }
    setUpdatingSignatureRequestId(requestID)
    try {
      const data = await updateDealSignatureRequestStatus(selectedDealId, requestID, status)
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setDetailForm(dealFormValues(data.deal))
      setLineItems(data.lineItems || [])
      setLineTotals(data.totals || emptyLineTotals)
      setSignatureRequests(data.signatureRequests || [])
      setActivities(data.activities || [])
      setError('')
    } catch (signatureError) {
      setError(signatureError.message || 'Unable to update proposal tracking.')
    } finally {
      setUpdatingSignatureRequestId(null)
    }
  }

  async function applyOwnerFilter(value) {
    setOwnerFilter(value)
    navigate(buildDealsPath(selectedDealId, search, pipelineFilter, stageFilter, value), { replace: true })
    await reloadDeals(search, pipelineFilter, stageFilter, value)
  }

  async function handleOwnerFilterChange(event) {
    await applyOwnerFilter(event.target.value)
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
    setSelectedDealId(null)
    navigate(buildDealsPath(null, nextSearch, nextPipelineFilter, nextStageFilter, nextOwnerFilter, nextCloseFrom, nextCloseTo), { replace: true })
    await reloadDeals(nextSearch, nextPipelineFilter, nextStageFilter, nextOwnerFilter, nextCloseFrom, nextCloseTo)
  }

  function handleOpenDealTasks() {
    if (!selectedDealId) {
      return
    }
    navigate(`/tasks?entityType=deal&entityId=${selectedDealId}`)
  }

  return (
    <section className="dashboard-grid contacts-grid">
      <Card>
        <div className="card-stack">
            <div className="section-header">
              <div>
                <h2>{labels.collection}</h2>
                <p>Real pipeline, real stages, no fake dashboard filler.</p>
              </div>
              <a className="button button-secondary" href={dealsExportURL({ search, pipelineId: pipelineFilter === 'all' ? 0 : Number.parseInt(pipelineFilter, 10) || 0, stageId: stageFilter === 'all' ? 0 : Number.parseInt(stageFilter, 10) || 0, ownerUserId: ownerFilter === 'all' ? 0 : Number.parseInt(ownerFilter, 10) || 0, closeFrom, closeTo })}>
                Export CSV
              </a>
            </div>
          <div className="record-list" role="list" aria-label="Pipeline summary">
            <article className="record-row" role="listitem">
              <div>
                <p>{labels.summaryOpen}</p>
              </div>
              <div>
                  <p>{meta.openCount}</p>
                </div>
              </article>
              <article className="record-row" role="listitem">
                <div>
                  <p>{labels.summaryWon}</p>
                </div>
                <div>
                  <p>{meta.wonCount}</p>
              </div>
            </article>
            <article className="record-row" role="listitem">
              <div>
                <p>Pipeline value</p>
              </div>
              <div>
                <p>{formatMoney(meta.pipelineValue, meta.currency)}</p>
                {(meta.missingRateCurrencies || []).length > 0 ? <p className="field-hint">Missing rates: {meta.missingRateCurrencies.join(', ')}</p> : null}
              </div>
            </article>
          </div>
          <Field label={labels.searchLabel}>
            <input className="text-input" type="search" value={search} onChange={handleSearchChange} />
          </Field>
          <SavedViews entityType="deals" currentFilters={{ q: search, pipeline: pipelineFilter, stage: stageFilter, owner: ownerFilter, closeFrom, closeTo }} onApply={handleApplySavedView} defaultName={`${labels.singular} view`} />
          <Field label="Pipeline filter">
            <select className="text-input" value={pipelineFilter} onChange={handlePipelineFilterChange}>
              <option value="all">All pipelines</option>
              {pipelines.map((pipeline) => (
                <option key={pipeline.id} value={pipeline.id}>{pipeline.name}</option>
              ))}
            </select>
          </Field>
          <Field label="Stage filter">
            <select className="text-input" value={stageFilter} onChange={handleStageFilterChange}>
              <option value="all">All stages</option>
              {filteredStages.map((stage) => (
                <option key={stage.id} value={stage.id}>{stageLabel(stage, pipelineFilter)}</option>
              ))}
            </select>
          </Field>
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
          <form className="auth-form" onSubmit={handleCloseDateFilter}>
            <Field label="Expected close from"><input className="text-input" type="date" value={closeFrom} onChange={(event) => setCloseFrom(event.target.value)} /></Field>
            <Field label="Expected close to"><input className="text-input" type="date" value={closeTo} onChange={(event) => setCloseTo(event.target.value)} /></Field>
            <Button className="button-secondary" type="submit">Apply close dates</Button>
          </form>
          {isListLoading ? <p className="field-hint">Loading {labels.showingLabel}...</p> : null}
          {error ? (
            <InlineError message={error} onRetry={() => reloadDeals(search, pipelineFilter, stageFilter, ownerFilter)} retryLabel={`Retry ${labels.showingLabel}`} />
          ) : null}
          {canWrite ? <BulkActions entityType="deal" selectedIds={selectedDealIds} visibleIds={deals.map((deal) => deal.id)} onSelectionChange={setSelectedDealIds} onChanged={() => reloadDeals(search, pipelineFilter, stageFilter, ownerFilter)} statuses={[]} userOptions={userOptions} /> : null}
          <div className="record-list" role="list" aria-label={labels.listAria}>
            {!isListLoading && deals.length === 0 ? (
              <EmptyState
                title={emptyDealsMessage(search, pipelineFilter, stageFilter, ownerFilter, labels, closeFrom, closeTo)}
                description={emptyDealsDescription(search, pipelineFilter, stageFilter, ownerFilter, labels, closeFrom, closeTo)}
                actionLabel={hasDealFilters ? 'Clear filters' : ''}
                onAction={() => {
                  if (hasDealFilters) {
                    setSearch('')
                    setPipelineFilter('all')
                    setStageFilter('all')
                    setOwnerFilter('all')
                    setCloseFrom('')
                    setCloseTo('')
                    navigate(buildDealsPath(null, '', 'all', 'all', 'all', '', ''), { replace: true })
                    reloadDeals('', 'all', 'all', 'all', '', '')
                  }
                }}
              />
            ) : deals.map((deal) => (
              <article className="record-row" key={deal.id} role="listitem">
                <div>
                  {canWrite ? <input type="checkbox" aria-label={`Select ${deal.name}`} checked={selectedDealIds.includes(deal.id)} onChange={() => setSelectedDealIds((current) => current.includes(deal.id) ? current.filter((id) => id !== deal.id) : [...current, deal.id])} /> : null}
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleSelectDeal(deal)}>
                    {deal.name}
                  </button>
                  <p>{deal.pipelineName ? `${deal.pipelineName} · ${deal.stageName}` : deal.stageName}</p>
                </div>
              <div>
                <p>{formatMoney(deal.valueAmount, deal.valueCurrency)}</p>
                <p>{deal.companyName || labels.companyEmpty}</p>
                <p className="field-hint">{deal.ownerUserName || 'Unassigned'}</p>
              </div>
              </article>
            ))}
          </div>
          <p className="field-hint">Showing {deals.length} of {meta.total} {labels.showingLabel}.</p>
        </div>
      </Card>

      {canWrite ? (
        <Card>
          <div className="card-stack">
            <div>
              <h2>{labels.createHeading}</h2>
              <p>{labels.createDescription}</p>
            </div>
            <form className="auth-form" onSubmit={handleCreate}>
              <Field label={`${labels.singular} name`}>
                <input className="text-input" value={form.name} onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))} required />
              </Field>
              <Field label="Stage">
                <select className="text-input" value={form.stageId} onChange={(event) => setForm((current) => ({ ...current, stageId: event.target.value, ...emptyCloseReview }))}>
                  {filteredStages.map((stage) => (
                    <option key={stage.id} value={stage.id}>{stageLabel(stage, pipelineFilter)}</option>
                  ))}
                </select>
              </Field>
              <CloseReviewFields outcome={stageOutcome(createStage)} value={form} onChange={setForm} />
              <Field label={labels.companyLabel}>
                <select className="text-input" value={form.companyId} onChange={(event) => setForm((current) => ({ ...current, companyId: event.target.value }))}>
                  <option value="">{labels.companyEmpty}</option>
                  {companyOptions.map((company) => (
                    <option key={company.id} value={company.id}>{company.name}</option>
                  ))}
                </select>
              </Field>
              <Field label={labels.contactLabel}>
                <select className="text-input" value={form.primaryContactId} onChange={(event) => setForm((current) => ({ ...current, primaryContactId: event.target.value }))}>
                  <option value="">{labels.contactEmpty}</option>
                  {contactOptions.map((contact) => (
                    <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>
                  ))}
                </select>
              </Field>
              <Field label={labels.valueLabel}>
                <input className="text-input" value={form.valueAmount} onChange={(event) => setForm((current) => ({ ...current, valueAmount: event.target.value }))} />
              </Field>
              <Field label="Value currency">
                <input className="text-input" value={form.valueCurrency} onChange={(event) => setForm((current) => ({ ...current, valueCurrency: event.target.value }))} />
              </Field>
              <Field label={labels.dateLabel}>
                <input className="text-input" type="date" value={form.expectedCloseDate} onChange={(event) => setForm((current) => ({ ...current, expectedCloseDate: event.target.value }))} />
              </Field>
              <Field label="Owner">
                <select className="text-input" value={form.ownerUserId} onChange={(event) => setForm((current) => ({ ...current, ownerUserId: event.target.value }))}>
                  {userOptions.map((user) => (
                    <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                  ))}
                </select>
              </Field>
              <Button type="submit">{`Save ${labels.singular.toLowerCase()}`}</Button>
            </form>
          </div>
        </Card>
      ) : null}

      {selectedDeal ? (
        <Card>
          <div className="card-stack">
            {isDetailLoading ? <p className="field-hint">Loading {labels.singular.toLowerCase()} detail...</p> : null}
            <div className="section-header">
              <div>
                <h2>{selectedDeal.name}</h2>
                <p>{selectedDeal.companyName || labels.companyEmpty}</p>
              </div>
              <div className="button-row">
                <a className="button button-secondary" href={quotePDFURL(selectedDealId)}>Download current quote PDF</a>
                {canWrite ? (
                  <Button className="button-danger" onClick={handleArchive}>
                    {labels.archiveAction}
                  </Button>
                ) : null}
              </div>
            </div>
            <DealCloseSummary deal={selectedDeal} />
            <form className="auth-form" aria-label="Deal details form" onSubmit={handleUpdate}>
              <Field label={`${labels.singular} name`}>
                <input className="text-input" value={detailForm.name} onChange={(event) => setDetailForm((current) => ({ ...current, name: event.target.value }))} required />
              </Field>
              <Field label={labels.companyLabel}>
                <select className="text-input" value={detailForm.companyId} onChange={(event) => setDetailForm((current) => ({ ...current, companyId: event.target.value }))}>
                  <option value="">{labels.companyEmpty}</option>
                  {companyOptions.map((company) => (
                    <option key={company.id} value={company.id}>{company.name}</option>
                  ))}
                </select>
              </Field>
              <Field label={labels.contactLabel}>
                <select className="text-input" value={detailForm.primaryContactId} onChange={(event) => setDetailForm((current) => ({ ...current, primaryContactId: event.target.value }))}>
                  <option value="">{labels.contactEmpty}</option>
                  {contactOptions.map((contact) => (
                    <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>
                  ))}
                </select>
              </Field>
              <Field label={labels.valueLabel}>
                <input className="text-input" value={detailForm.valueAmount} onChange={(event) => setDetailForm((current) => ({ ...current, valueAmount: event.target.value }))} />
              </Field>
              <Field label="Value currency">
                <input className="text-input" value={detailForm.valueCurrency} onChange={(event) => setDetailForm((current) => ({ ...current, valueCurrency: event.target.value }))} />
              </Field>
              <Field label={labels.dateLabel}>
                <input className="text-input" type="date" value={detailForm.expectedCloseDate} onChange={(event) => setDetailForm((current) => ({ ...current, expectedCloseDate: event.target.value }))} />
              </Field>
              <Field label="Owner">
                <select className="text-input" value={detailForm.ownerUserId} onChange={(event) => setDetailForm((current) => ({ ...current, ownerUserId: event.target.value }))}>
                  {userOptions.map((user) => (
                    <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
                  ))}
                </select>
              </Field>
              {canWrite ? <Button type="submit">{`Update ${labels.singular.toLowerCase()}`}</Button> : null}
            </form>
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
            {canWrite ? (
              <>
                <Field label={labels.moveLabel}>
                  <select className="text-input" value={selectedStageId} onChange={(event) => { setSelectedStageId(event.target.value); setStageCloseReview(emptyCloseReview) }}>
                    {stages.map((stage) => (
                      <option key={stage.id} value={stage.id}>{stageLabel(stage, 'all')}</option>
                    ))}
                  </select>
                </Field>
                <CloseReviewFields outcome={stageOutcome(moveStage)} value={stageCloseReview} onChange={setStageCloseReview} />
                <Button type="button" onClick={handleMoveStage}>{labels.moveAction}</Button>
              </>
            ) : null}
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

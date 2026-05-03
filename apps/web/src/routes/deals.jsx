import { useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { EmptyState } from '../components/ui/empty_state'
import { SavedViews } from '../components/ui/saved_views'
import { ActivityTimeline } from '../components/ui/activity_timeline'
import { InlineError } from '../components/ui/inline_error'
import { archiveDeal, createDeal, dealsExportURL, getDeal, listDeals, listDealStages, updateDeal, updateDealStage } from '../lib/deals'
import { createNote, listNotes } from '../lib/notes'
import { createTask, listTasks } from '../lib/tasks'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listOrganizationUsers } from '../lib/users'
import { isAbortError } from '../lib/api'
import { useAuth } from '../app/providers'
import { usePageTitle } from '../lib/use_page_title'

const emptyForm = {
  name: '',
  stageId: '',
  companyId: '',
  primaryContactId: '',
  status: 'open',
  valueAmount: '',
  valueCurrency: 'USD',
  expectedCloseDate: '',
  ownerUserId: ''
}

const emptyTaskForm = {
  title: '',
  description: '',
  dueAt: '',
  assignedToUserId: ''
}

function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD' }).format(amount)
}

function dealFormValues(deal) {
  return {
    name: deal.name || '',
    stageId: deal.stageId ? String(deal.stageId) : '',
    companyId: deal.companyId ? String(deal.companyId) : '',
    primaryContactId: deal.primaryContactId ? String(deal.primaryContactId) : '',
    status: deal.status || 'open',
    valueAmount: deal.valueAmount || '',
    valueCurrency: deal.valueCurrency || 'USD',
    expectedCloseDate: deal.expectedCloseDate || '',
    ownerUserId: deal.ownerUserId ? String(deal.ownerUserId) : ''
  }
}

function pipelineLabels(businessType) {
  if (businessType === 'services' || businessType === 'construction-services') {
    return {
      collection: 'Jobs',
      singular: 'Job',
      createHeading: 'New job',
      createDescription: 'Create jobs against the real org stage list.',
      summaryOpen: 'Open jobs',
      summaryWon: 'Won jobs',
      searchLabel: 'Search jobs',
      companyLabel: 'Client',
      companyEmpty: 'No client linked',
      contactLabel: 'Primary contact',
      contactEmpty: 'No primary contact',
      valueLabel: 'Job value',
      dateLabel: 'Target date',
      showingLabel: 'jobs',
      listAria: 'Jobs list',
      notesAria: 'Job notes list',
      tasksAria: 'Job tasks list',
      activityAria: 'Job activity list',
      archiveAction: 'Archive job',
      moveAction: 'Move job to stage',
      moveLabel: 'Move job stage'
    }
  }

  return {
    collection: 'Deals',
    singular: 'Deal',
    createHeading: 'New deal',
    createDescription: 'Create pipeline entries against the real org stage list.',
    summaryOpen: 'Open deals',
    summaryWon: 'Won deals',
    searchLabel: 'Search deals',
    companyLabel: 'Company',
    companyEmpty: 'No company linked',
    contactLabel: 'Primary contact',
    contactEmpty: 'No primary contact',
    valueLabel: 'Value amount',
    dateLabel: 'Expected close date',
    showingLabel: 'deals',
    listAria: 'Deals list',
    notesAria: 'Deal notes list',
    tasksAria: 'Deal tasks list',
    activityAria: 'Deal activity list',
    archiveAction: 'Archive deal',
    moveAction: 'Move to stage',
    moveLabel: 'Move stage'
  }
}

function emptyDealsMessage(search, stageFilter, ownerFilter, labels) {
  if (search.trim() || stageFilter !== 'all' || ownerFilter !== 'all') {
    return `No ${labels.showingLabel} match the current filters.`
  }

  return `No ${labels.showingLabel} yet.`
}

function emptyDealsDescription(search, stageFilter, ownerFilter, labels) {
  if (search.trim() || stageFilter !== 'all' || ownerFilter !== 'all') {
    return 'Clear a filter or try a broader search to see more pipeline records.'
  }

  return `Create the first ${labels.singular.toLowerCase()} once you have a real opportunity, job, or follow-up conversation to track.`
}

export function DealsRoute() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const { dealId } = useParams()
  const { session, businessProfile } = useAuth()
  const routeDealId = Number.parseInt(dealId || '', 10)
  const businessType = businessProfile?.businessType || session?.organization?.businessType || 'general'
  const currentUserId = session?.user?.id ? String(session.user.id) : ''
  const labels = pipelineLabels(businessType)
  usePageTitle(labels.collection)
  const initialSearch = searchParams.get('q') || ''
  const initialStageFilter = searchParams.get('stage') || 'all'
  const initialOwnerFilter = searchParams.get('owner') || 'all'
  const initialCompanyId = searchParams.get('companyId') || ''
  const initialPrimaryContactId = searchParams.get('primaryContactId') || ''
  const [stages, setStages] = useState([])
  const [deals, setDeals] = useState([])
  const [meta, setMeta] = useState({ page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' })
  const [form, setForm] = useState({
    ...emptyForm,
    companyId: initialCompanyId,
    primaryContactId: initialPrimaryContactId
  })
  const [detailForm, setDetailForm] = useState(emptyForm)
  const [search, setSearch] = useState(initialSearch)
  const [stageFilter, setStageFilter] = useState(initialStageFilter)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [companyOptions, setCompanyOptions] = useState([])
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [selectedDealId, setSelectedDealId] = useState(null)
  const [selectedStageId, setSelectedStageId] = useState('')
  const [notes, setNotes] = useState([])
  const [tasks, setTasks] = useState([])
  const [noteBody, setNoteBody] = useState('')
  const [taskForm, setTaskForm] = useState(emptyTaskForm)
  const [activities, setActivities] = useState([])
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const [pipelineReady, setPipelineReady] = useState(false)
  const listControllerRef = useRef(null)
  const hasDealFilters = search.trim() !== '' || stageFilter !== 'all' || ownerFilter !== 'all'

  function buildDealsPath(nextDealId = routeDealId, nextSearch = search, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter) {
    const params = new URLSearchParams()
    if (nextSearch) {
      params.set('q', nextSearch)
    }
    if (nextStageFilter !== 'all') {
      params.set('stage', nextStageFilter)
    }
    if (nextOwnerFilter !== 'all') {
      params.set('owner', nextOwnerFilter)
    }
    const suffix = params.toString() ? `?${params.toString()}` : ''
    const pathname = nextDealId ? `/deals/${nextDealId}` : '/deals'
    return `${pathname}${suffix}`
  }

  async function loadDeals(nextSearch = search, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, { signal } = {}) {
    const isUnassigned = nextOwnerFilter === 'unassigned'
    const loadedDeals = await listDeals({
      search: nextSearch,
      stageId: nextStageFilter === 'all' ? 0 : Number.parseInt(nextStageFilter, 10) || 0,
      unassigned: isUnassigned,
      ownerUserId: isUnassigned || nextOwnerFilter === 'all' ? 0 : Number.parseInt(nextOwnerFilter, 10) || 0
    }, { signal })
    setDeals(loadedDeals.deals || [])
    setMeta(loadedDeals.meta || { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' })
  }

  async function loadPipeline(nextSearch = search, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, { signal } = {}) {
    const isUnassigned = nextOwnerFilter === 'unassigned'
    const [loadedStages, loadedDeals, loadedCompanies, loadedContacts, loadedUsers] = await Promise.all([
      listDealStages({ signal }),
      listDeals({
        search: nextSearch,
        stageId: nextStageFilter === 'all' ? 0 : Number.parseInt(nextStageFilter, 10) || 0,
        unassigned: isUnassigned,
        ownerUserId: isUnassigned || nextOwnerFilter === 'all' ? 0 : Number.parseInt(nextOwnerFilter, 10) || 0
      }, { signal }),
      listCompanies('', { signal }),
      listContacts('', { signal }),
      listOrganizationUsers({ signal })
    ])
    setStages(loadedStages)
    setDeals(loadedDeals.deals || [])
    setCompanyOptions(loadedCompanies.companies || [])
    setContactOptions(loadedContacts.contacts || [])
    setUserOptions(loadedUsers)
    setMeta(loadedDeals.meta || { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' })
    if (loadedStages.length > 0 && !selectedStageId) {
      setSelectedStageId(String(loadedStages[0].id))
    }
    setForm((current) => ({
      ...current,
      stageId: current.stageId || (loadedStages[0] ? String(loadedStages[0].id) : ''),
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
        await loadPipeline(initialSearch, initialStageFilter, initialOwnerFilter, { signal: controller.signal })
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
  }, [initialCompanyId, initialOwnerFilter, initialPrimaryContactId, initialSearch, initialStageFilter])

  const selectedDeal = useMemo(() => deals.find((entry) => entry.id === selectedDealId) || null, [deals, selectedDealId])

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
          setNotes([])
          setTasks([])
          setActivities([])
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
        const [dealData, loadedNotes, taskData] = await Promise.all([
          getDeal(routeDealId, { signal: controller.signal }),
          listNotes('deal', routeDealId, { signal: controller.signal }),
          listTasks({ status: 'open', entityType: 'deal', entityId: routeDealId }, { signal: controller.signal })
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

  async function reloadDeals(nextSearch = search, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter) {
    listControllerRef.current?.abort()
    const controller = new AbortController()
    listControllerRef.current = controller
    setIsListLoading(true)
    try {
      await loadDeals(nextSearch, nextStageFilter, nextOwnerFilter, { signal: controller.signal })
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
    navigate(buildDealsPath(selectedDealId, value, stageFilter, ownerFilter), { replace: true })
    await reloadDeals(value, stageFilter, ownerFilter)
  }

  async function handleStageFilterChange(event) {
    const value = event.target.value
    setStageFilter(value)
    navigate(buildDealsPath(selectedDealId, search, value, ownerFilter), { replace: true })
    await reloadDeals(search, value, ownerFilter)
  }

  async function handleCreate(event) {
    event.preventDefault()
    try {
      const data = await createDeal({
        name: form.name,
        stageId: Number.parseInt(form.stageId, 10),
        companyId: Number.parseInt(form.companyId, 10) || 0,
        primaryContactId: Number.parseInt(form.primaryContactId, 10) || 0,
        status: form.status,
        valueAmount: form.valueAmount,
        valueCurrency: form.valueCurrency,
        expectedCloseDate: form.expectedCloseDate,
        ownerUserId: Number.parseInt(form.ownerUserId, 10) || 0
      })
      setDeals((current) => [...current, data.deal])
      setNotes(data.notes || [])
      setTasks(data.tasks || [])
      setNoteBody('')
      setTaskForm(emptyTaskForm)
      setActivities(data.activities || [])
      setSelectedDealId(data.deal.id)
      setSelectedStageId(String(data.deal.stageId))
      setDetailForm(dealFormValues(data.deal))
      setMeta((current) => ({
        ...current,
        total: current.total + 1,
        openCount: current.openCount + 1,
        pipelineValue: String(Number.parseFloat(current.pipelineValue || '0') + Number.parseFloat(data.deal.valueAmount || '0'))
      }))
      setForm((current) => ({ ...emptyForm, stageId: current.stageId || form.stageId || (stages[0] ? String(stages[0].id) : '') }))
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

    try {
      const data = await updateDealStage(selectedDealId, Number.parseInt(selectedStageId, 10))
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setActivities(data.activities || [])
      setError('')
    } catch (moveError) {
      setError(moveError.message || 'Unable to move deal.')
    }
  }

  async function handleSelectDeal(deal, { signal } = {}) {
    setSelectedDealId(deal.id)
    setSelectedStageId(String(deal.stageId))
    setDetailForm(dealFormValues(deal))
    setActivities([])
    setNoteBody('')
    setTaskForm(emptyTaskForm)
    navigate(buildDealsPath(deal.id))
    try {
      const [loadedNotes, taskData] = await Promise.all([
        listNotes('deal', deal.id, { signal }),
        listTasks({ status: 'open', entityType: 'deal', entityId: deal.id }, { signal })
      ])
      if (signal?.aborted) {
        return
      }
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
        status: detailForm.status,
        valueAmount: detailForm.valueAmount,
        valueCurrency: detailForm.valueCurrency,
        expectedCloseDate: detailForm.expectedCloseDate,
        ownerUserId: Number.parseInt(detailForm.ownerUserId, 10) || 0
      })
      setDeals((current) => current.map((entry) => (entry.id === selectedDealId ? data.deal : entry)))
      setDetailForm(dealFormValues(data.deal))
      setActivities(data.activities || [])
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
      setDetailForm(emptyForm)
      setNotes([])
      setTasks([])
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

  async function applyOwnerFilter(value) {
    setOwnerFilter(value)
    navigate(buildDealsPath(selectedDealId, search, stageFilter, value), { replace: true })
    await reloadDeals(search, stageFilter, value)
  }

  async function handleOwnerFilterChange(event) {
    await applyOwnerFilter(event.target.value)
  }

  async function handleApplySavedView(filters) {
    const nextSearch = filters.q || ''
    const nextStageFilter = filters.stage || 'all'
    const nextOwnerFilter = filters.owner || 'all'
    setSearch(nextSearch)
    setStageFilter(nextStageFilter)
    setOwnerFilter(nextOwnerFilter)
    setSelectedDealId(null)
    navigate(buildDealsPath(null, nextSearch, nextStageFilter, nextOwnerFilter), { replace: true })
    await reloadDeals(nextSearch, nextStageFilter, nextOwnerFilter)
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
              <a className="button button-secondary" href={dealsExportURL({ search, stageId: stageFilter === 'all' ? 0 : Number.parseInt(stageFilter, 10) || 0, ownerUserId: ownerFilter === 'all' ? 0 : Number.parseInt(ownerFilter, 10) || 0 })}>
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
                <p>{formatMoney(meta.pipelineValue)}</p>
              </div>
            </article>
          </div>
          <Field label={labels.searchLabel}>
            <input className="text-input" type="search" value={search} onChange={handleSearchChange} />
          </Field>
          <SavedViews entityType="deals" currentFilters={{ q: search, stage: stageFilter, owner: ownerFilter }} onApply={handleApplySavedView} defaultName={`${labels.singular} view`} />
          <Field label="Stage filter">
            <select className="text-input" value={stageFilter} onChange={handleStageFilterChange}>
              <option value="all">All stages</option>
              {stages.map((stage) => (
                <option key={stage.id} value={stage.id}>{stage.name}</option>
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
          {isListLoading ? <p className="field-hint">Loading {labels.showingLabel}...</p> : null}
          {error ? (
            <InlineError message={error} onRetry={() => reloadDeals(search, stageFilter, ownerFilter)} retryLabel={`Retry ${labels.showingLabel}`} />
          ) : null}
          <div className="record-list" role="list" aria-label={labels.listAria}>
            {!isListLoading && deals.length === 0 ? (
              <EmptyState
                title={emptyDealsMessage(search, stageFilter, ownerFilter, labels)}
                description={emptyDealsDescription(search, stageFilter, ownerFilter, labels)}
                actionLabel={hasDealFilters ? 'Clear filters' : ''}
                onAction={() => {
                  if (hasDealFilters) {
                    setSearch('')
                    setStageFilter('all')
                    setOwnerFilter('all')
                    navigate(buildDealsPath(null, '', 'all', 'all'), { replace: true })
                    reloadDeals('', 'all', 'all')
                  }
                }}
              />
            ) : deals.map((deal) => (
              <article className="record-row" key={deal.id} role="listitem">
                <div>
                  <button className="button button-ghost contact-link" type="button" onClick={() => handleSelectDeal(deal)}>
                    {deal.name}
                  </button>
                  <p>{deal.stageName}</p>
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
              <select className="text-input" value={form.stageId} onChange={(event) => setForm((current) => ({ ...current, stageId: event.target.value }))}>
                {stages.map((stage) => (
                  <option key={stage.id} value={stage.id}>{stage.name}</option>
                ))}
              </select>
            </Field>
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

      {selectedDeal ? (
        <Card>
          <div className="card-stack">
            {isDetailLoading ? <p className="field-hint">Loading {labels.singular.toLowerCase()} detail...</p> : null}
            <div className="section-header">
              <div>
                <h2>{selectedDeal.name}</h2>
                <p>{selectedDeal.companyName || labels.companyEmpty}</p>
              </div>
              <Button className="button-danger" onClick={handleArchive}>
                {labels.archiveAction}
              </Button>
            </div>
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
              <Field label="Status">
                <select className="text-input" value={detailForm.status} onChange={(event) => setDetailForm((current) => ({ ...current, status: event.target.value }))}>
                  <option value="open">Open</option>
                  <option value="won">Won</option>
                  <option value="lost">Lost</option>
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
              <Button type="submit">{`Update ${labels.singular.toLowerCase()}`}</Button>
            </form>
            <Field label={labels.moveLabel}>
              <select className="text-input" value={selectedStageId} onChange={(event) => setSelectedStageId(event.target.value)}>
                {stages.map((stage) => (
                  <option key={stage.id} value={stage.id}>{stage.name}</option>
                ))}
              </select>
            </Field>
            <Button onClick={handleMoveStage}>{labels.moveAction}</Button>
            <Card>
              <div className="card-stack">
                <h3>Notes</h3>
                <form className="auth-form" onSubmit={handleCreateNote}>
                  <Field label="New note">
                    <textarea className="text-input" value={noteBody} onChange={(event) => setNoteBody(event.target.value)} rows={4} />
                  </Field>
                  <Button type="submit">Add note</Button>
                </form>
                <div className="record-list" role="list" aria-label={labels.notesAria}>
                  {notes.map((note) => (
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
                  <Button className="button-secondary" type="button" onClick={handleOpenDealTasks}>
                    Open in tasks
                  </Button>
                </div>
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
                <div className="record-list" role="list" aria-label={labels.tasksAria}>
                  {tasks.map((task) => (
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
                <ActivityTimeline activities={activities} ariaLabel={labels.activityAria} />
              </div>
            </Card>
          </div>
        </Card>
      ) : null}
    </section>
  )
}

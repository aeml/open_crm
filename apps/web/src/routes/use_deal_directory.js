import { useEffect, useRef, useState } from 'react'
import { isAbortError } from '../lib/api'
import { listCompanies } from '../lib/companies'
import { listContacts } from '../lib/contacts'
import { listDeals, listDealPipelines } from '../lib/deals'
import { listOrganizationUsers } from '../lib/users'
import { emptyDealMeta, flattenPipelineStages, stagesForPipeline } from './deal_view'

export function dealDirectoryPath({ closeFrom = '', closeTo = '', dealId = null, owner = 'all', pipeline = 'all', search = '', stage = 'all' } = {}) {
  const params = new URLSearchParams()
  if (search) params.set('q', search)
  if (pipeline !== 'all') params.set('pipeline', pipeline)
  if (stage !== 'all') params.set('stage', stage)
  if (owner !== 'all') params.set('owner', owner)
  if (closeFrom) params.set('closeFrom', closeFrom)
  if (closeTo) params.set('closeTo', closeTo)
  const suffix = params.toString() ? `?${params.toString()}` : ''
  return `${dealId ? `/deals/${dealId}` : '/deals'}${suffix}`
}

export function useDealDirectory({
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
}) {
  const [pipelines, setPipelines] = useState([])
  const [stages, setStages] = useState([])
  const [deals, setDeals] = useState([])
  const [selectedDealIds, setSelectedDealIds] = useState([])
  const [meta, setMeta] = useState(emptyDealMeta)
  const [form, setForm] = useState(initialForm)
  const [search, setSearch] = useState(initialSearch)
  const [pipelineFilter, setPipelineFilter] = useState(initialPipelineFilter)
  const [stageFilter, setStageFilter] = useState(initialStageFilter)
  const [ownerFilter, setOwnerFilter] = useState(initialOwnerFilter)
  const [closeFrom, setCloseFrom] = useState(initialCloseFrom)
  const [closeTo, setCloseTo] = useState(initialCloseTo)
  const [companyOptions, setCompanyOptions] = useState([])
  const [contactOptions, setContactOptions] = useState([])
  const [userOptions, setUserOptions] = useState([])
  const [error, setError] = useState('')
  const [isListLoading, setIsListLoading] = useState(true)
  const [pipelineReady, setPipelineReady] = useState(false)
  const listControllerRef = useRef(null)
  const listRequestRef = useRef(null)

  function buildDealsPath(
    nextDealId = routeDealId,
    nextSearch = search,
    nextPipelineFilter = pipelineFilter,
    nextStageFilter = stageFilter,
    nextOwnerFilter = ownerFilter,
    nextCloseFrom = closeFrom,
    nextCloseTo = closeTo
  ) {
    return dealDirectoryPath({
      closeFrom: nextCloseFrom,
      closeTo: nextCloseTo,
      dealId: nextDealId,
      owner: nextOwnerFilter,
      pipeline: nextPipelineFilter,
      search: nextSearch,
      stage: nextStageFilter
    })
  }

  function applyDeals(loadedDeals) {
    setDeals(loadedDeals.deals || [])
    setSelectedDealIds([])
    setMeta(loadedDeals.meta || emptyDealMeta)
  }

  async function loadDeals(nextSearch = search, nextPipelineFilter = pipelineFilter, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, { request = {}, signal, nextCloseFrom = closeFrom, nextCloseTo = closeTo } = {}) {
    listRequestRef.current = request
    const isUnassigned = nextOwnerFilter === 'unassigned'
    try {
      const loadedDeals = await listDeals({
        search: nextSearch,
        pipelineId: nextPipelineFilter === 'all' ? 0 : Number.parseInt(nextPipelineFilter, 10) || 0,
        stageId: nextStageFilter === 'all' ? 0 : Number.parseInt(nextStageFilter, 10) || 0,
        unassigned: isUnassigned,
        ownerUserId: isUnassigned || nextOwnerFilter === 'all' ? 0 : Number.parseInt(nextOwnerFilter, 10) || 0,
        closeFrom: nextCloseFrom,
        closeTo: nextCloseTo
      }, { signal })
      if (listRequestRef.current !== request || signal?.aborted) return false
      applyDeals(loadedDeals)
      return true
    } catch (loadError) {
      if (listRequestRef.current !== request || signal?.aborted || isAbortError(loadError)) return false
      throw loadError
    }
  }

  async function reloadDeals(nextSearch = search, nextPipelineFilter = pipelineFilter, nextStageFilter = stageFilter, nextOwnerFilter = ownerFilter, nextCloseFrom = closeFrom, nextCloseTo = closeTo) {
    listControllerRef.current?.abort()
    const controller = new AbortController()
    const request = {}
    listControllerRef.current = controller
    setIsListLoading(true)
    try {
      const applied = await loadDeals(nextSearch, nextPipelineFilter, nextStageFilter, nextOwnerFilter, { request, signal: controller.signal, nextCloseFrom, nextCloseTo })
      if (applied) setError('')
    } catch (loadError) {
      if (!isAbortError(loadError) && listRequestRef.current === request) {
        setError(loadError.message || 'Unable to load deals.')
      }
    } finally {
      if (listRequestRef.current === request) setIsListLoading(false)
      if (listControllerRef.current === controller) listControllerRef.current = null
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    const request = {}
    listRequestRef.current = request

    async function run() {
      setIsListLoading(true)
      try {
        const isUnassigned = initialOwnerFilter === 'unassigned'
        const [loadedPipelines, loadedDeals, loadedCompanies, loadedContacts, loadedUsers] = await Promise.all([
          listDealPipelines({ signal: controller.signal }),
          listDeals({
            search: initialSearch,
            pipelineId: initialPipelineFilter === 'all' ? 0 : Number.parseInt(initialPipelineFilter, 10) || 0,
            stageId: initialStageFilter === 'all' ? 0 : Number.parseInt(initialStageFilter, 10) || 0,
            unassigned: isUnassigned,
            ownerUserId: isUnassigned || initialOwnerFilter === 'all' ? 0 : Number.parseInt(initialOwnerFilter, 10) || 0,
            closeFrom: initialCloseFrom,
            closeTo: initialCloseTo
          }, { signal: controller.signal }),
          listCompanies('', { signal: controller.signal }),
          listContacts('', { signal: controller.signal }),
          listOrganizationUsers({ signal: controller.signal })
        ])
        if (controller.signal.aborted) return
        const loadedStages = flattenPipelineStages(loadedPipelines)
        const nextStages = stagesForPipeline(loadedStages, initialPipelineFilter)
        setStages(loadedStages)
        setPipelines(loadedPipelines)
        setCompanyOptions(loadedCompanies.companies || [])
        setContactOptions(loadedContacts.contacts || [])
        setUserOptions(loadedUsers)
        setForm((current) => ({
          ...current,
          stageId: current.stageId || (nextStages[0] ? String(nextStages[0].id) : ''),
          companyId: current.companyId || initialCompanyId || (loadedCompanies.companies?.[0] ? String(loadedCompanies.companies[0].id) : ''),
          primaryContactId: current.primaryContactId || initialPrimaryContactId || (loadedContacts.contacts?.[0] ? String(loadedContacts.contacts[0].id) : ''),
          ownerUserId: current.ownerUserId || (loadedUsers[0] ? String(loadedUsers[0].id) : '')
        }))
        setPipelineReady(true)
        if (listRequestRef.current === request) {
          applyDeals(loadedDeals)
          setError('')
        }
      } catch (loadError) {
        if (!controller.signal.aborted && !isAbortError(loadError)) {
          setPipelineReady(true)
          setError(loadError.message || 'Unable to load deals.')
        }
      } finally {
        if (listRequestRef.current === request) setIsListLoading(false)
      }
    }

    run()
    return () => {
      controller.abort()
      listControllerRef.current?.abort()
      listRequestRef.current = null
    }
  }, [])

  useEffect(() => {
    const alreadySynchronized = search === initialSearch &&
      pipelineFilter === initialPipelineFilter && stageFilter === initialStageFilter &&
      ownerFilter === initialOwnerFilter && closeFrom === initialCloseFrom && closeTo === initialCloseTo
    if (alreadySynchronized) return

    setSearch(initialSearch)
    setPipelineFilter(initialPipelineFilter)
    setStageFilter(initialStageFilter)
    setOwnerFilter(initialOwnerFilter)
    setCloseFrom(initialCloseFrom)
    setCloseTo(initialCloseTo)
    reloadDeals(initialSearch, initialPipelineFilter, initialStageFilter, initialOwnerFilter, initialCloseFrom, initialCloseTo)
  }, [initialCloseFrom, initialCloseTo, initialOwnerFilter, initialPipelineFilter, initialSearch, initialStageFilter])

  return {
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
  }
}

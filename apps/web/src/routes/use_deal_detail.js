import { useEffect, useMemo, useState } from 'react'
import { isAbortError } from '../lib/api'
import { getDeal } from '../lib/deals'
import { listProductCatalogItems } from '../lib/product_catalog'
import { loadQuotePreparation } from '../lib/quote_templates'
import { emptyCloseReview } from './deal_close_review'
import { dealFormValues } from './deal_view'
import { useDealCommercials } from './use_deal_commercials'
import { requireDealResponse, useDealSelection } from './use_deal_selection'
import { useDealWork } from './use_deal_work'

const emptyDetailForm = {
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

export function useDealDetail({ deals, navigateToDeal, pipelineReady, routeDealId, setDeals, setError, userOptions }) {
  const [selectedDealId, setSelectedDealId] = useState(null)
  const [selectedStageId, setSelectedStageId] = useState('')
  const [stageCloseReview, setStageCloseReview] = useState(emptyCloseReview)
  const [detailForm, setDetailForm] = useState(emptyDetailForm)
  const [isDetailLoading, setIsDetailLoading] = useState(false)
  const selection = useDealSelection(selectedDealId)
  const work = useDealWork({
    defaultAssignedToUserId: userOptions[0]?.id ? String(userOptions[0].id) : '',
    selectedDealId,
    selection,
    onError: setError
  })
  const commercial = useDealCommercials({ selectedDealId, selection, onDealUpdated: applyCommercialDealUpdate, onError: setError })
  const selectedDeal = useMemo(() => deals.find((entry) => entry.id === selectedDealId) || null, [deals, selectedDealId])

  function applyCommercialDealUpdate(data, dealId, isCurrent) {
    setDeals((current) => current.map((entry) => (entry.id === dealId ? data.deal : entry)))
    if (!isCurrent) return
    setDetailForm(dealFormValues(data.deal))
    work.setActivities(data.activities || [])
  }

  function clear() {
    selection.clear()
    setSelectedDealId(null)
    setSelectedStageId('')
    setStageCloseReview(emptyCloseReview)
    setDetailForm(emptyDetailForm)
    work.reset()
    commercial.reset()
    setIsDetailLoading(false)
  }

  async function select(deal) {
    const activeSelection = selection.begin(deal.id)
    const signal = activeSelection.controller.signal
    setSelectedDealId(deal.id)
    setSelectedStageId(String(deal.stageId))
    setStageCloseReview(emptyCloseReview)
    setDetailForm(dealFormValues(deal))
    work.reset()
    commercial.reset()
    setIsDetailLoading(true)
    navigateToDeal(deal.id)
    try {
      const [dealData, loadedWork, loadedCatalog, quotePreparation] = await Promise.all([
        getDeal(deal.id, { signal }),
        work.fetchWork(deal.id, { signal }),
        listProductCatalogItems({ signal }),
        loadQuotePreparation({ signal })
      ])
      if (!selection.isCurrent(activeSelection)) return
      requireDealResponse(dealData, deal.id, 'Unable to load deal.')
      setDeals((current) => current.map((entry) => (entry.id === deal.id ? dealData.deal : entry)))
      setDetailForm(dealFormValues(dealData.deal))
      work.load({ ...loadedWork, activities: dealData.activities || [] })
      commercial.load(dealData, loadedCatalog, quotePreparation)
      setError('')
    } catch (loadError) {
      if (!isAbortError(loadError) && selection.isCurrent(activeSelection)) {
        work.reset()
        setError(loadError.message || 'Unable to load deal.')
      }
    } finally {
      if (selection.isCurrent(activeSelection)) setIsDetailLoading(false)
    }
  }

  useEffect(() => {
    async function syncRouteDeal() {
      if (!pipelineReady) return

      if (!Number.isInteger(routeDealId) || routeDealId <= 0) {
        if (selectedDealId) clear()
        return
      }

      if (selectedDealId === routeDealId) return

      const routeDeal = deals.find((entry) => entry.id === routeDealId)
      if (routeDeal) {
        await select(routeDeal)
        return
      }

      const activeSelection = selection.begin(routeDealId)
      const signal = activeSelection.controller.signal
      setSelectedDealId(routeDealId)
      setSelectedStageId('')
      setStageCloseReview(emptyCloseReview)
      setDetailForm(emptyDetailForm)
      work.reset()
      commercial.reset()
      try {
        setIsDetailLoading(true)
        const [dealData, loadedWork, loadedCatalog, quotePreparation] = await Promise.all([
          getDeal(routeDealId, { signal }),
          work.fetchWork(routeDealId, { signal }),
          listProductCatalogItems({ signal }),
          loadQuotePreparation({ signal })
        ])
        if (!selection.isCurrent(activeSelection)) return
        requireDealResponse(dealData, routeDealId, 'Unable to load deal.')
        setDeals((current) => {
          const next = current.filter((entry) => entry.id !== routeDealId)
          return [dealData.deal, ...next]
        })
        setSelectedDealId(dealData.deal.id)
        setSelectedStageId(String(dealData.deal.stageId))
        setDetailForm(dealFormValues(dealData.deal))
        work.load({ ...loadedWork, activities: dealData.activities || [] })
        commercial.load(dealData, loadedCatalog, quotePreparation)
        setError('')
      } catch (loadError) {
        if (!isAbortError(loadError) && selection.isCurrent(activeSelection)) {
          setError(loadError.message || 'Unable to load deal.')
        }
      } finally {
        if (selection.isCurrent(activeSelection)) setIsDetailLoading(false)
      }
    }

    syncRouteDeal()
  }, [deals, pipelineReady, routeDealId, selectedDealId])

  return {
    clear,
    commercial,
    detailForm,
    isDetailLoading,
    select,
    selectedDeal,
    selectedDealId,
    selectedStageId,
    selection,
    setDetailForm,
    setSelectedDealId,
    setSelectedStageId,
    setStageCloseReview,
    stageCloseReview,
    work
  }
}

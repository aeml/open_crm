import { useRef, useState } from 'react'
import {
  createDealSignatureRequest,
  finalizeDealQuote,
  replaceDealLineItems,
  updateDealSignatureRequestStatus
} from '../lib/deals'
import { createIdempotencyKey } from '../lib/idempotency'
import {
  emptyLineItemForm,
  emptyLineTotals,
  emptyQuoteForm,
  emptySignatureForm,
  lineItemFormFromCatalogItem,
  lineItemPayload
} from './deal_view'
import { requireDealResponse } from './use_deal_selection'

// useDealCommercials keeps quote-line and manual proposal-tracking state and
// mutations together. The route remains responsible for selecting a deal and
// applying the returned deal/activity snapshot to its directory and timeline.
export function useDealCommercials({ selectedDealId, selection, onDealUpdated, onError }) {
  const [productCatalogItems, setProductCatalogItems] = useState([])
  const [lineItems, setLineItems] = useState([])
  const [lineItemForm, setLineItemForm] = useState(emptyLineItemForm)
  const [lineTotals, setLineTotals] = useState(emptyLineTotals)
  const [quotes, setQuotes] = useState([])
  const [quoteForm, setQuoteForm] = useState(() => emptyQuoteForm())
  const [signatureRequests, setSignatureRequests] = useState([])
  const [signatureForm, setSignatureForm] = useState(emptySignatureForm)
  const [isSavingLineItems, setIsSavingLineItems] = useState(false)
  const [isFinalizingQuote, setIsFinalizingQuote] = useState(false)
  const [areLineItemsDirty, setAreLineItemsDirty] = useState(false)
  const [isCreatingSignatureRequest, setIsCreatingSignatureRequest] = useState(false)
  const [updatingSignatureRequestId, setUpdatingSignatureRequestId] = useState(null)
  const quoteAttempt = useRef(null)
  const isSnapshotPending = isSavingLineItems || isFinalizingQuote || isCreatingSignatureRequest || updatingSignatureRequestId !== null

  function refresh(data) {
    setLineItems(data.lineItems || [])
    setLineTotals(data.totals || emptyLineTotals)
    setQuotes(data.quotes || [])
    setSignatureRequests(data.signatureRequests || [])
  }

  function load(data, catalogItems) {
    refresh(data)
    if (catalogItems) {
      setProductCatalogItems(catalogItems)
    }
    setLineItemForm(emptyLineItemForm)
    setQuoteForm(emptyQuoteForm(data.deal?.primaryContactName || ''))
    setAreLineItemsDirty(false)
    quoteAttempt.current = null
    setSignatureForm(emptySignatureForm)
    setIsSavingLineItems(false)
    setIsFinalizingQuote(false)
    setIsCreatingSignatureRequest(false)
    setUpdatingSignatureRequestId(null)
  }

  function reset() {
    setLineItems([])
    setLineTotals(emptyLineTotals)
    setLineItemForm(emptyLineItemForm)
    setQuotes([])
    setQuoteForm(emptyQuoteForm())
    setAreLineItemsDirty(false)
    quoteAttempt.current = null
    setSignatureRequests([])
    setSignatureForm(emptySignatureForm)
    setIsSavingLineItems(false)
    setIsFinalizingQuote(false)
    setIsCreatingSignatureRequest(false)
    setUpdatingSignatureRequestId(null)
  }

  function handleCatalogLineItemChange(event) {
    const productCatalogItemId = event.target.value
    const catalogItem = productCatalogItems.find((item) => String(item.id) === productCatalogItemId)
    setLineItemForm(lineItemFormFromCatalogItem(catalogItem))
  }

  function handleAddLineItem(event) {
    event.preventDefault()
    if (!lineItemForm.name.trim()) {
      onError('Line item name is required.')
      return
    }
    setLineItems((current) => [...current, { ...lineItemForm, position: current.length + 1 }])
    setAreLineItemsDirty(true)
    setLineItemForm(emptyLineItemForm)
    onError('')
  }

  function handleRemoveLineItem(index) {
    setLineItems((current) => current
      .filter((_, entryIndex) => entryIndex !== index)
      .map((item, entryIndex) => ({ ...item, position: entryIndex + 1 })))
    setAreLineItemsDirty(true)
  }

  async function handleSaveLineItems() {
    const operation = selection.start('line-items', selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    setIsSavingLineItems(true)
    try {
      const data = requireDealResponse(
        await replaceDealLineItems(operation.dealId, { items: lineItems.map(lineItemPayload) }),
        operation.dealId,
        'Unable to update deal line items.'
      )
      const isCurrent = selection.isCurrent(operation.selection)
      if (selection.canApply(operation)) onDealUpdated(data, operation.dealId, isCurrent)
      if (isCurrent) {
        refresh(data)
        setAreLineItemsDirty(false)
        onError('')
      }
    } catch (lineItemError) {
      if (selection.isCurrent(operation.selection)) {
        onError(lineItemError.message || 'Unable to update deal line items.')
      }
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsSavingLineItems(false)
    }
  }

  async function handleFinalizeQuote(event) {
    event.preventDefault()
    const operation = selection.start('quote-finalize', selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    const input = {
      recipientName: quoteForm.recipientName.trim(),
      recipientEmail: quoteForm.recipientEmail.trim(),
      validUntil: quoteForm.validUntil,
      terms: quoteForm.terms.trim()
    }
    const fingerprint = JSON.stringify(input)
    if (quoteAttempt.current?.fingerprint !== fingerprint) {
      quoteAttempt.current = { fingerprint, key: createIdempotencyKey('quote') }
    }
    setIsFinalizingQuote(true)
    try {
      const quote = await finalizeDealQuote(operation.dealId, input, quoteAttempt.current.key)
      if (!quote?.id) throw new Error('Unable to finalize quote.')
      if (selection.isCurrent(operation.selection)) {
        setQuotes((current) => [quote, ...current.filter((entry) => entry.id !== quote.id)])
        setQuoteForm(emptyQuoteForm(quote.recipientName))
        quoteAttempt.current = null
        onError('')
      }
    } catch (quoteError) {
      if (selection.isCurrent(operation.selection)) onError(quoteError.message || 'Unable to finalize quote.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsFinalizingQuote(false)
    }
  }

  async function handleCreateSignatureRequest(event) {
    event.preventDefault()
    const operation = selection.start('signature-create', selectedDealId, { group: 'deal-snapshot' })
    if (!operation || !signatureForm.signerName.trim() || !signatureForm.signerEmail.trim()) {
      selection.finish(operation)
      return
    }
    setIsCreatingSignatureRequest(true)
    try {
      const data = requireDealResponse(await createDealSignatureRequest(operation.dealId, {
        signerName: signatureForm.signerName.trim(), signerEmail: signatureForm.signerEmail.trim()
      }), operation.dealId, 'Unable to create proposal tracking.')
      const isCurrent = selection.isCurrent(operation.selection)
      if (selection.canApply(operation)) onDealUpdated(data, operation.dealId, isCurrent)
      if (isCurrent) {
        refresh(data)
        setSignatureForm(emptySignatureForm)
        onError('')
      }
    } catch (signatureError) {
      if (selection.isCurrent(operation.selection)) {
        onError(signatureError.message || 'Unable to create proposal tracking.')
      }
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setIsCreatingSignatureRequest(false)
    }
  }

  async function handleUpdateSignatureRequestStatus(requestID, status) {
    const operation = selection.start(`signature-${requestID}`, selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    setUpdatingSignatureRequestId(requestID)
    try {
      const data = requireDealResponse(
        await updateDealSignatureRequestStatus(operation.dealId, requestID, status),
        operation.dealId,
        'Unable to update proposal tracking.'
      )
      const isCurrent = selection.isCurrent(operation.selection)
      if (selection.canApply(operation)) onDealUpdated(data, operation.dealId, isCurrent)
      if (isCurrent) {
        refresh(data)
        onError('')
      }
    } catch (signatureError) {
      if (selection.isCurrent(operation.selection)) {
        onError(signatureError.message || 'Unable to update proposal tracking.')
      }
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setUpdatingSignatureRequestId(null)
    }
  }

  return {
    handleAddLineItem,
    handleCatalogLineItemChange,
    handleCreateSignatureRequest,
    handleFinalizeQuote,
    handleRemoveLineItem,
    handleSaveLineItems,
    handleUpdateSignatureRequestStatus,
    isCreatingSignatureRequest,
    isFinalizingQuote,
    isSavingLineItems,
    isSnapshotPending,
    lineItemForm,
    lineItems,
    lineTotals,
    areLineItemsDirty,
    load,
    productCatalogItems,
    refresh,
    reset,
    quoteForm,
    quotes,
    setLineItemForm,
    setQuoteForm,
    setSignatureForm,
    signatureForm,
    signatureRequests,
    updatingSignatureRequestId
  }
}

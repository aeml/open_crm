import { useRef, useState } from 'react'
import {
  deliverDealQuote,
  finalizeDealQuote,
  replaceDealLineItems,
  resolveDealQuoteDelivery,
  voidDealSignatureRequest
} from '../lib/deals'
import { createIdempotencyKey } from '../lib/idempotency'
import {
  emptyLineItemForm,
  emptyLineTotals,
  emptyQuoteForm,
  lineItemFormFromCatalogItem,
  lineItemPayload
} from './deal_view'
import { requireDealResponse } from './use_deal_selection'

// useDealCommercials keeps quote-line and immutable signature-request state and
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
  const [isSavingLineItems, setIsSavingLineItems] = useState(false)
  const [isFinalizingQuote, setIsFinalizingQuote] = useState(false)
  const [deliveringQuoteId, setDeliveringQuoteId] = useState(null)
  const [resolvingDeliveryId, setResolvingDeliveryId] = useState(null)
  const [areLineItemsDirty, setAreLineItemsDirty] = useState(false)
  const [voidingSignatureRequestId, setVoidingSignatureRequestId] = useState(null)
  const quoteAttempt = useRef(null)
  const deliveryAttempts = useRef(new Map())
  const isSnapshotPending = isSavingLineItems || isFinalizingQuote || voidingSignatureRequestId !== null

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
    deliveryAttempts.current.clear()
    setIsSavingLineItems(false)
    setIsFinalizingQuote(false)
    setDeliveringQuoteId(null)
    setResolvingDeliveryId(null)
    setVoidingSignatureRequestId(null)
  }

  function reset() {
    setLineItems([])
    setLineTotals(emptyLineTotals)
    setLineItemForm(emptyLineItemForm)
    setQuotes([])
    setQuoteForm(emptyQuoteForm())
    setAreLineItemsDirty(false)
    quoteAttempt.current = null
    deliveryAttempts.current.clear()
    setSignatureRequests([])
    setIsSavingLineItems(false)
    setIsFinalizingQuote(false)
    setDeliveringQuoteId(null)
    setResolvingDeliveryId(null)
    setVoidingSignatureRequestId(null)
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

  function replaceQuoteDelivery(quoteID, delivery) {
    setQuotes((current) => current.map((quote) => {
      if (quote.id !== quoteID) return quote
      const deliveries = quote.deliveries || []
      return { ...quote, deliveries: [delivery, ...deliveries.filter((entry) => entry.id !== delivery.id)] }
    }))
  }

  function replaceSignatureFromDelivery(quote, delivery) {
    if (!delivery.signatureRequestId) return
    const status = delivery.status === 'sent' ? 'sent' : delivery.status === 'failed' ? 'voided' : 'draft'
    setSignatureRequests((current) => {
      const existing = current.find((entry) => entry.id === delivery.signatureRequestId)
      if (existing && ['signed', 'declined', 'voided'].includes(existing.status)) return current
      const request = {
        ...(existing || {}),
        id: delivery.signatureRequestId,
        quoteId: quote.id,
        deliveryId: delivery.id,
        quoteNumber: quote.quoteNumber,
        signerName: quote.recipientName,
        signerEmail: quote.recipientEmail,
        status,
        provider: 'open_crm_native',
        quoteFileName: quote.pdfFilename,
        sentAt: delivery.sentAt || '',
        createdAt: existing?.createdAt || delivery.createdAt,
        updatedAt: delivery.updatedAt
      }
      return [request, ...current.filter((entry) => entry.id !== request.id)]
    })
  }

  async function handleDeliverQuote(quote, input) {
    const operation = selection.start(`quote-delivery-${quote.id}`, selectedDealId)
    if (!operation) return
    const payload = { subject: input.subject.trim(), messageBody: input.messageBody.trim(), requestSignature: Boolean(input.requestSignature) }
    const fingerprint = JSON.stringify(payload)
    const prior = deliveryAttempts.current.get(quote.id)
    if (prior?.fingerprint !== fingerprint) {
      deliveryAttempts.current.set(quote.id, { fingerprint, key: createIdempotencyKey('quote-delivery') })
    }
    setDeliveringQuoteId(quote.id)
    try {
      const delivery = await deliverDealQuote(operation.dealId, quote.id, payload, deliveryAttempts.current.get(quote.id).key)
      if (!delivery?.id) throw new Error('Unable to deliver quote.')
      if (selection.isCurrent(operation.selection)) {
        replaceQuoteDelivery(quote.id, delivery)
        replaceSignatureFromDelivery(quote, delivery)
        if (delivery.status === 'sent' || delivery.status === 'failed') deliveryAttempts.current.delete(quote.id)
        onError('')
      }
    } catch (deliveryError) {
      if (selection.isCurrent(operation.selection)) onError(deliveryError.message || 'Unable to deliver quote.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setDeliveringQuoteId(null)
    }
  }

  async function handleResolveQuoteDelivery(quoteID, deliveryID, resolution) {
    const operation = selection.start(`quote-delivery-resolve-${deliveryID}`, selectedDealId)
    if (!operation) return
    setResolvingDeliveryId(deliveryID)
    try {
      const delivery = await resolveDealQuoteDelivery(deliveryID, resolution)
      if (!delivery?.id) throw new Error('Unable to resolve quote delivery.')
      if (selection.isCurrent(operation.selection)) {
        replaceQuoteDelivery(quoteID, delivery)
        const quote = quotes.find((entry) => entry.id === quoteID)
        if (quote) replaceSignatureFromDelivery(quote, delivery)
        if (delivery.status === 'sent' || delivery.status === 'failed') deliveryAttempts.current.delete(quoteID)
        onError('')
      }
    } catch (resolutionError) {
      if (selection.isCurrent(operation.selection)) onError(resolutionError.message || 'Unable to resolve quote delivery.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setResolvingDeliveryId(null)
    }
  }

  async function handleVoidSignatureRequest(requestID) {
    const operation = selection.start(`signature-void-${requestID}`, selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    setVoidingSignatureRequestId(requestID)
    try {
      const data = requireDealResponse(
        await voidDealSignatureRequest(operation.dealId, requestID),
        operation.dealId,
        'Unable to void signature request.'
      )
      const isCurrent = selection.isCurrent(operation.selection)
      if (selection.canApply(operation)) onDealUpdated(data, operation.dealId, isCurrent)
      if (isCurrent) {
        refresh(data)
        onError('')
      }
    } catch (signatureError) {
      if (selection.isCurrent(operation.selection)) {
        onError(signatureError.message || 'Unable to void signature request.')
      }
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setVoidingSignatureRequestId(null)
    }
  }

  return {
    handleAddLineItem,
    handleCatalogLineItemChange,
    handleFinalizeQuote,
    handleDeliverQuote,
    handleResolveQuoteDelivery,
    handleRemoveLineItem,
    handleSaveLineItems,
    handleVoidSignatureRequest,
    isFinalizingQuote,
    deliveringQuoteId,
    resolvingDeliveryId,
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
    signatureRequests,
    voidingSignatureRequestId
  }
}

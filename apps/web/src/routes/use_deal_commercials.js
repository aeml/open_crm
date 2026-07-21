import { useRef, useState } from 'react'
import {
  convertSignedQuoteToWon,
  deliverDealQuote,
  finalizeDealQuote,
  reissueExpiredDealQuote,
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
  const [reissuingQuoteId, setReissuingQuoteId] = useState(null)
  const [resolvingDeliveryId, setResolvingDeliveryId] = useState(null)
  const [areLineItemsDirty, setAreLineItemsDirty] = useState(false)
  const [voidingSignatureRequestId, setVoidingSignatureRequestId] = useState(null)
  const [convertingSignatureRequestId, setConvertingSignatureRequestId] = useState(null)
  const idempotencyAttempts = useRef(new Map())
  const isSnapshotPending = isSavingLineItems || isFinalizingQuote || reissuingQuoteId !== null || voidingSignatureRequestId !== null || convertingSignatureRequestId !== null

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
    idempotencyAttempts.current.clear()
    setIsSavingLineItems(false)
    setIsFinalizingQuote(false)
    setDeliveringQuoteId(null)
    setReissuingQuoteId(null)
    setResolvingDeliveryId(null)
    setVoidingSignatureRequestId(null)
    setConvertingSignatureRequestId(null)
  }

  function reset() {
    setLineItems([])
    setLineTotals(emptyLineTotals)
    setLineItemForm(emptyLineItemForm)
    setQuotes([])
    setQuoteForm(emptyQuoteForm())
    setAreLineItemsDirty(false)
    idempotencyAttempts.current.clear()
    setSignatureRequests([])
    setIsSavingLineItems(false)
    setIsFinalizingQuote(false)
    setDeliveringQuoteId(null)
    setReissuingQuoteId(null)
    setResolvingDeliveryId(null)
    setVoidingSignatureRequestId(null)
    setConvertingSignatureRequestId(null)
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

  function idempotencyKey(name, payload, prefix) {
    const fingerprint = JSON.stringify(payload)
    const prior = idempotencyAttempts.current.get(name)
    if (prior?.fingerprint !== fingerprint) {
      idempotencyAttempts.current.set(name, { fingerprint, key: createIdempotencyKey(prefix) })
    }
    return idempotencyAttempts.current.get(name).key
  }

  async function mutateSnapshot(operationName, pendingID, setPending, request, message, onSuccess) {
    const operation = selection.start(operationName, selectedDealId, { group: 'deal-snapshot' })
    if (!operation) return
    setPending(pendingID)
    try {
      const data = requireDealResponse(await request(operation.dealId), operation.dealId, message)
      const isCurrent = selection.isCurrent(operation.selection)
      if (selection.canApply(operation)) onDealUpdated(data, operation.dealId, isCurrent)
      if (isCurrent) {
        refresh(data)
        onSuccess?.()
        onError('')
      }
    } catch (error) {
      if (selection.isCurrent(operation.selection)) onError(error.message || message)
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setPending(null)
    }
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
    const attemptName = 'quote'
    setIsFinalizingQuote(true)
    try {
      const quote = await finalizeDealQuote(operation.dealId, input, idempotencyKey(attemptName, input, 'quote'))
      if (!quote?.id) throw new Error('Unable to finalize quote.')
      if (selection.isCurrent(operation.selection)) {
        setQuotes((current) => [quote, ...current.filter((entry) => entry.id !== quote.id)])
        setQuoteForm(emptyQuoteForm(quote.recipientName))
        idempotencyAttempts.current.delete(attemptName)
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
    const attemptName = `delivery-${quote.id}`
    setDeliveringQuoteId(quote.id)
    try {
      const delivery = await deliverDealQuote(operation.dealId, quote.id, payload, idempotencyKey(attemptName, payload, 'quote-delivery'))
      if (!delivery?.id) throw new Error('Unable to deliver quote.')
      if (selection.isCurrent(operation.selection)) {
        replaceQuoteDelivery(quote.id, delivery)
        replaceSignatureFromDelivery(quote, delivery)
        if (delivery.status === 'sent' || delivery.status === 'failed') idempotencyAttempts.current.delete(attemptName)
        onError('')
      }
    } catch (deliveryError) {
      if (selection.isCurrent(operation.selection)) onError(deliveryError.message || 'Unable to deliver quote.')
    } finally {
      selection.finish(operation)
      if (selection.isCurrent(operation.selection)) setDeliveringQuoteId(null)
    }
  }

  async function handleReissueQuote(quote, validUntil) {
    const payload = { validUntil }
    const attemptName = `quote-reissue-${quote.id}`
    return mutateSnapshot(
      attemptName,
      quote.id,
      setReissuingQuoteId,
      (dealID) => reissueExpiredDealQuote(dealID, quote.id, payload, idempotencyKey(attemptName, payload, 'quote-reissue')),
      'Unable to reissue expired quote.',
      () => idempotencyAttempts.current.delete(attemptName)
    )
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
        if (delivery.status === 'sent' || delivery.status === 'failed') idempotencyAttempts.current.delete(`delivery-${quoteID}`)
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
    return mutateSnapshot(
      `signature-void-${requestID}`,
      requestID,
      setVoidingSignatureRequestId,
      (dealID) => voidDealSignatureRequest(dealID, requestID),
      'Unable to void signature request.'
    )
  }

  async function handleConvertSignatureRequest(requestID, input) {
    const payload = {
      stageId: Number.parseInt(input.stageId, 10),
      closeReasonCode: input.closeReasonCode,
      closeNotes: input.closeNotes.trim()
    }
    const attemptName = `signature-convert-${requestID}`
    return mutateSnapshot(
      attemptName,
      requestID,
      setConvertingSignatureRequestId,
      (dealID) => convertSignedQuoteToWon(dealID, requestID, payload, idempotencyKey(attemptName, payload, 'signed-quote-conversion')),
      'Unable to convert signed quote to a won deal.',
      () => idempotencyAttempts.current.delete(attemptName)
    )
  }

  return {
    handleAddLineItem,
    handleCatalogLineItemChange,
    handleConvertSignatureRequest,
    handleFinalizeQuote,
    handleDeliverQuote,
    handleReissueQuote,
    handleResolveQuoteDelivery,
    handleRemoveLineItem,
    handleSaveLineItems,
    handleVoidSignatureRequest,
    isFinalizingQuote,
    convertingSignatureRequestId,
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

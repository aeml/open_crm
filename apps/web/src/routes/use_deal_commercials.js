import { useState } from 'react'
import {
  createDealSignatureRequest,
  replaceDealLineItems,
  updateDealSignatureRequestStatus
} from '../lib/deals'
import {
  emptyLineItemForm,
  emptyLineTotals,
  emptySignatureForm,
  lineItemFormFromCatalogItem,
  lineItemPayload
} from './deal_view'

// useDealCommercials keeps quote-line and manual proposal-tracking state and
// mutations together. The route remains responsible for selecting a deal and
// applying the returned deal/activity snapshot to its directory and timeline.
export function useDealCommercials({ selectedDealId, onDealUpdated, onError }) {
  const [productCatalogItems, setProductCatalogItems] = useState([])
  const [lineItems, setLineItems] = useState([])
  const [lineItemForm, setLineItemForm] = useState(emptyLineItemForm)
  const [lineTotals, setLineTotals] = useState(emptyLineTotals)
  const [signatureRequests, setSignatureRequests] = useState([])
  const [signatureForm, setSignatureForm] = useState(emptySignatureForm)
  const [isSavingLineItems, setIsSavingLineItems] = useState(false)
  const [isCreatingSignatureRequest, setIsCreatingSignatureRequest] = useState(false)
  const [updatingSignatureRequestId, setUpdatingSignatureRequestId] = useState(null)

  function refresh(data) {
    setLineItems(data.lineItems || [])
    setLineTotals(data.totals || emptyLineTotals)
    setSignatureRequests(data.signatureRequests || [])
  }

  function load(data, catalogItems) {
    refresh(data)
    if (catalogItems) {
      setProductCatalogItems(catalogItems)
    }
    setLineItemForm(emptyLineItemForm)
    setSignatureForm(emptySignatureForm)
  }

  function reset() {
    setLineItems([])
    setLineTotals(emptyLineTotals)
    setLineItemForm(emptyLineItemForm)
    setSignatureRequests([])
    setSignatureForm(emptySignatureForm)
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
    setLineItemForm(emptyLineItemForm)
    onError('')
  }

  function handleRemoveLineItem(index) {
    setLineItems((current) => current
      .filter((_, entryIndex) => entryIndex !== index)
      .map((item, entryIndex) => ({ ...item, position: entryIndex + 1 })))
  }

  async function handleSaveLineItems() {
    if (!selectedDealId) {
      return
    }
    setIsSavingLineItems(true)
    try {
      const data = await replaceDealLineItems(selectedDealId, { items: lineItems.map(lineItemPayload) })
      refresh(data)
      onDealUpdated(data)
      onError('')
    } catch (lineItemError) {
      onError(lineItemError.message || 'Unable to update deal line items.')
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
      refresh(data)
      setSignatureForm(emptySignatureForm)
      onDealUpdated(data)
      onError('')
    } catch (signatureError) {
      onError(signatureError.message || 'Unable to create proposal tracking.')
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
      refresh(data)
      onDealUpdated(data)
      onError('')
    } catch (signatureError) {
      onError(signatureError.message || 'Unable to update proposal tracking.')
    } finally {
      setUpdatingSignatureRequestId(null)
    }
  }

  return {
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
    load,
    productCatalogItems,
    refresh,
    reset,
    setLineItemForm,
    setSignatureForm,
    signatureForm,
    signatureRequests,
    updatingSignatureRequestId
  }
}

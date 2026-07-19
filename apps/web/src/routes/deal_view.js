export const emptyLineItemForm = {
  productCatalogItemId: '',
  name: '',
  sku: '',
  itemType: 'product',
  quantity: '1',
  unitName: 'unit',
  unitPrice: '0.00',
  discountAmount: '0.00',
  taxRate: '0',
  currency: 'USD'
}

export const emptyLineTotals = { subtotal: '0', discountTotal: '0', taxTotal: '0', total: '0', currency: 'USD' }

export const emptyDealMeta = { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0', currency: 'USD', missingRateCurrencies: [] }

export const emptySignatureForm = {
  signerName: '',
  signerEmail: ''
}

export function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  const normalizedCurrency = String(currency || 'USD').toUpperCase()
  const safeCurrency = /^[A-Z]{3}$/.test(normalizedCurrency) ? normalizedCurrency : 'USD'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: safeCurrency }).format(amount)
}

export function formatSignatureTime(value) {
  if (!value) {
    return 'Not recorded'
  }
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not recorded' : date.toLocaleString()
}

export function signatureStatusLabel(status) {
  if (status === 'voided') return 'Voided'
  if (status === 'declined') return 'Declined'
  if (status === 'signed') return 'Signed'
  if (status === 'sent') return 'Sent'
  return 'Draft'
}

export function flattenPipelineStages(pipelines) {
  return pipelines.flatMap((pipeline) => (pipeline.stages || []).map((stage) => ({ ...stage, pipelineId: stage.pipelineId || pipeline.id, pipelineName: pipeline.name })))
}

export function stagesForPipeline(stages, pipelineFilter) {
  if (pipelineFilter === 'all') {
    return stages
  }
  return stages.filter((stage) => String(stage.pipelineId) === String(pipelineFilter))
}

export function stageLabel(stage, pipelineFilter) {
  if (pipelineFilter === 'all' && stage.pipelineName) {
    return `${stage.pipelineName}: ${stage.name}`
  }
  return stage.name
}

export function dealFormValues(deal) {
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

export function lineItemFormFromCatalogItem(item) {
  if (!item) {
    return emptyLineItemForm
  }
  return {
    productCatalogItemId: String(item.id),
    name: item.name || '',
    sku: item.sku || '',
    itemType: item.itemType || 'product',
    quantity: '1',
    unitName: item.unitName || 'unit',
    unitPrice: item.unitPrice || '0.00',
    discountAmount: '0.00',
    taxRate: '0',
    currency: item.currency || 'USD'
  }
}

export function lineItemPayload(item, index) {
  return {
    productCatalogItemId: Number.parseInt(item.productCatalogItemId, 10) || 0,
    name: item.name,
    sku: item.sku,
    itemType: item.itemType,
    quantity: item.quantity,
    unitName: item.unitName,
    unitPrice: item.unitPrice,
    discountAmount: item.discountAmount,
    taxRate: item.taxRate,
    currency: item.currency,
    position: index + 1
  }
}

export function pipelineLabels(businessType) {
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

export function emptyDealsMessage(search, pipelineFilter, stageFilter, ownerFilter, labels, closeFrom = '', closeTo = '') {
  if (search.trim() || pipelineFilter !== 'all' || stageFilter !== 'all' || ownerFilter !== 'all' || closeFrom || closeTo) {
    return `No ${labels.showingLabel} match the current filters.`
  }

  return `No ${labels.showingLabel} yet.`
}

export function emptyDealsDescription(search, pipelineFilter, stageFilter, ownerFilter, labels, closeFrom = '', closeTo = '') {
  if (search.trim() || pipelineFilter !== 'all' || stageFilter !== 'all' || ownerFilter !== 'all' || closeFrom || closeTo) {
    return 'Clear a filter or try a broader search to see more pipeline records.'
  }

  return `Create the first ${labels.singular.toLowerCase()} once you have a real opportunity, job, or follow-up conversation to track.`
}

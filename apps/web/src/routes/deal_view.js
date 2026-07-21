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

export function quoteValidUntil(defaultValidityDays = 30) {
  const validUntil = new Date()
  validUntil.setUTCDate(validUntil.getUTCDate() + defaultValidityDays)
  return validUntil.toISOString().slice(0, 10)
}

export function emptyQuoteForm(recipientName = '', requestApproval = false) {
  return {
    recipientName,
    recipientEmail: '',
    validUntil: quoteValidUntil(),
    terms: 'Payment due within 30 days of invoice.',
    templateId: '',
    templateRevision: 0,
    requestApproval
  }
}

export function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  const normalized = String(currency || 'USD').toUpperCase()
  if (!Number.isFinite(amount)) return '$0.00'
  const safeCurrency = /^[A-Z]{3}$/.test(normalized) ? normalized : 'USD'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: safeCurrency }).format(amount)
}

export function quoteCurrencyDisclosure(quote) {
  return quote.fxDisclosure?.displayText || 'Legacy version: FX snapshot unavailable.'
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
  const jobs = businessType === 'services' || businessType === 'construction-services'
  const singular = jobs ? 'Job' : 'Deal'
  const lower = singular.toLowerCase()
  return {
    collection: `${singular}s`,
    singular,
    createHeading: `New ${lower}`,
    createDescription: jobs ? 'Create jobs against the real org stage list.' : 'Create pipeline entries against the real org stage list.',
    summaryOpen: `Open ${lower}s`,
    summaryWon: `Won ${lower}s`,
    searchLabel: `Search ${lower}s`,
    companyLabel: jobs ? 'Client' : 'Company',
    companyEmpty: jobs ? 'No client linked' : 'No company linked',
    contactLabel: 'Primary contact',
    contactEmpty: 'No primary contact',
    valueLabel: jobs ? 'Job value' : 'Value amount',
    dateLabel: jobs ? 'Target date' : 'Expected close date',
    showingLabel: `${lower}s`,
    listAria: `${singular}s list`,
    notesAria: `${singular} notes list`,
    tasksAria: `${singular} tasks list`,
    activityAria: `${singular} activity list`,
    archiveAction: `Archive ${lower}`,
    moveAction: jobs ? 'Move job to stage' : 'Move to stage',
    moveLabel: jobs ? 'Move job stage' : 'Move stage'
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

import { customFieldFormValues, customFieldPayload } from '../lib/custom_fields'

export const emptyLinkedPersonForm = {
  firstName: '',
  lastName: '',
  email: '',
  phone: '',
  jobTitle: '',
  status: 'lead',
  customFields: {}
}

export function parseLinkedContactIDs(value) {
  return String(value || '')
    .split(',')
    .map((entry) => Number.parseInt(entry.trim(), 10))
    .filter((entry) => Number.isInteger(entry) && entry > 0)
}

export function splitFullName(value) {
  const parts = String(value || '').trim().split(/\s+/).filter(Boolean)
  if (parts.length === 0) {
    return { firstName: '', lastName: '' }
  }
  if (parts.length === 1) {
    return { firstName: parts[0], lastName: parts[0] }
  }
  return {
    firstName: parts[0],
    lastName: parts.slice(1).join(' ')
  }
}

export function formatAddress(value = {}) {
  const street = [value.addressLine1, value.addressLine2].filter(Boolean).join(', ')
  const locality = [value.city, value.state, value.postalCode].filter(Boolean).join(', ')
  return [street, locality, value.country].filter(Boolean).join(' | ')
}

export function individualClientFromContact(contact) {
  return {
    id: `contact-${contact.id}`,
    entityId: contact.id,
    entityType: 'contact',
    clientType: 'individual',
    name: `${contact.firstName || ''} ${contact.lastName || ''}`.trim(),
    addressLine1: contact.addressLine1 || '',
    addressLine2: contact.addressLine2 || '',
    city: contact.city || '',
    state: contact.state || '',
    postalCode: contact.postalCode || '',
    country: contact.country || '',
    industry: contact.jobTitle || '',
    phone: contact.phone || '',
    website: '',
    status: contact.status || 'lead',
    email: contact.email || '',
    ownerUserName: contact.ownerUserName || ''
  }
}

export function organizationClientFromCompany(company) {
  return {
    ...company,
    entityId: company.id,
    entityType: 'company'
  }
}

export function buildClientRecords(companies, contacts) {
  return [
    ...(companies || []).map(organizationClientFromCompany),
    ...(contacts || []).filter((contact) => contact.isClient).map(individualClientFromContact)
  ].sort((left, right) => left.name.localeCompare(right.name) || left.entityId - right.entityId)
}

export function normalizeClientType(value) {
  return value === 'individual' ? 'individual' : 'organization'
}

export function isIndividualClient(clientType) {
  return normalizeClientType(clientType) === 'individual'
}

export function limitLinkedContacts(clientType, value) {
  const ids = parseLinkedContactIDs(value)
  if (isIndividualClient(clientType)) {
    return ids.slice(0, 1).join(',')
  }
  return ids.join(',')
}

export function buildCompanyPayload(form, definitions = [], { includeLinkedContacts = true } = {}) {
  const individual = isIndividualClient(form.clientType)
  return {
    name: form.name,
    clientType: normalizeClientType(form.clientType),
    addressLine1: form.addressLine1,
    addressLine2: form.addressLine2,
    city: form.city,
    state: form.state,
    postalCode: form.postalCode,
    country: form.country,
    industry: individual ? '' : form.industry,
    email: individual ? form.email : '',
    phone: form.phone,
    website: individual ? '' : form.website,
    status: form.status,
    ...(includeLinkedContacts ? { linkedContactIDs: parseLinkedContactIDs(form.linkedContactIDs) } : {}),
    customFields: customFieldPayload(definitions, form.customFields)
  }
}

export function companyFormValues(company, linkedContacts = [], definitions = []) {
  return {
    name: company.name || '',
    clientType: normalizeClientType(company.clientType),
    addressLine1: company.addressLine1 || '',
    addressLine2: company.addressLine2 || '',
    city: company.city || '',
    state: company.state || '',
    postalCode: company.postalCode || '',
    country: company.country || '',
    industry: company.industry || '',
    email: company.email || '',
    phone: company.phone || '',
    website: company.website || '',
    status: company.status || 'prospect',
    linkedContactIDs: limitLinkedContacts(company.clientType, (linkedContacts || []).map((contact) => contact.id).join(',')),
    customFields: customFieldFormValues(definitions, company.customFields)
  }
}

export function clientTypeLabel(clientType) {
  return isIndividualClient(clientType) ? 'Individual' : 'Organization'
}

export function linkedContactFieldLabel(clientType) {
  return isIndividualClient(clientType) ? 'Person record' : 'Initial linked contact'
}

export function linkedContactFieldHint(clientType) {
  return isIndividualClient(clientType)
    ? 'Individual clients need one linked person record.'
    : 'Optionally link one existing person now. Additional people are managed from the client record.'
}

export function createDescription(clientType) {
  return isIndividualClient(clientType)
    ? 'Add an individual client and link the matching person record.'
    : 'Add an organization client and tie the right contacts to it immediately.'
}

export function nameFieldLabel(clientType) {
  return isIndividualClient(clientType) ? 'Full name' : 'Client name'
}

export function phoneFieldLabel(clientType) {
  return isIndividualClient(clientType) ? 'Phone number' : 'Phone'
}

export function detailSubtitle(company, linkedContacts = []) {
  if (!company) {
    return ''
  }
  if (isIndividualClient(company.clientType)) {
    return company.email || (linkedContacts?.[0]?.email) || company.phone || company.status || 'Individual client'
  }
  return company.website || formatAddress(company) || company.status || ''
}

export function applyLinkedContactSelection(currentForm, contactOptions, value) {
  const nextLinkedContactIDs = limitLinkedContacts(currentForm.clientType, value)
  if (!isIndividualClient(currentForm.clientType)) {
    return { ...currentForm, linkedContactIDs: nextLinkedContactIDs }
  }

  const selectedID = parseLinkedContactIDs(nextLinkedContactIDs)[0] || 0
  const selectedContact = contactOptions.find((contact) => contact.id === selectedID)

  return {
    ...currentForm,
    linkedContactIDs: nextLinkedContactIDs,
    name: currentForm.name || `${selectedContact?.firstName || ''} ${selectedContact?.lastName || ''}`.trim(),
    email: currentForm.email || selectedContact?.email || '',
    phone: currentForm.phone || selectedContact?.phone || '',
    addressLine1: currentForm.addressLine1 || selectedContact?.addressLine1 || '',
    addressLine2: currentForm.addressLine2 || selectedContact?.addressLine2 || '',
    city: currentForm.city || selectedContact?.city || '',
    state: currentForm.state || selectedContact?.state || '',
    postalCode: currentForm.postalCode || selectedContact?.postalCode || '',
    country: currentForm.country || selectedContact?.country || ''
  }
}

export function linkedPersonFormValues(company) {
  const status = ['prospect', 'customer', 'lead'].includes(company?.status) ? company.status : 'lead'
  return {
    ...emptyLinkedPersonForm,
    status
  }
}

export function sortContactOptions(contacts) {
  return [...contacts].sort((left, right) => {
    const leftName = `${left.firstName || ''} ${left.lastName || ''}`.trim()
    const rightName = `${right.firstName || ''} ${right.lastName || ''}`.trim()
    return leftName.localeCompare(rightName) || left.id - right.id
  })
}

export function mergeLinkedContactIDs(linkedContacts, nextContactID) {
  const result = []
  const seen = new Set()

  for (const contact of linkedContacts || []) {
    const contactID = Number.parseInt(String(contact?.id || ''), 10)
    if (!Number.isInteger(contactID) || contactID <= 0 || seen.has(contactID)) {
      continue
    }
    seen.add(contactID)
    result.push(contactID)
  }

  if (Number.isInteger(nextContactID) && nextContactID > 0 && !seen.has(nextContactID)) {
    result.push(nextContactID)
  }

  return result
}

export function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) {
    return '$0.00'
  }
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD' }).format(amount)
}

export function relatedPipelineLabels(businessType) {
  if (businessType === 'services' || businessType === 'construction-services') {
    return { plural: 'Jobs', singular: 'job' }
  }
  if (businessType === 'product-sales') {
    return { plural: 'Opportunities', singular: 'opportunity' }
  }
  return { plural: 'Deals', singular: 'deal' }
}

export function primaryLinkedContactID(linkedContacts = []) {
  const primaryContact = linkedContacts.find((contact) => contact.isPrimary) || linkedContacts[0]
  return primaryContact?.id || 0
}

export function emailRecipientOptions(linkedContacts = []) {
  return linkedContacts
    .filter((contact) => contact.email)
    .map((contact) => {
      const name = `${contact.firstName || ''} ${contact.lastName || ''}`.trim() || contact.email
      return { id: contact.id, label: `${name} (${contact.email})` }
    })
}

export function duplicateSearchTerm(message, fallback = '') {
  const text = String(message || '')
  const marker = text.toLowerCase().lastIndexOf('duplicate company:')
  if (marker >= 0) {
    const candidate = text.slice(marker + 'duplicate company:'.length).split('(')[0].trim()
    if (candidate) {
      return candidate
    }
  }
  return String(fallback || '').trim()
}

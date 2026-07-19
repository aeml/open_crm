import { customFieldFormValues, customFieldPayload } from '../lib/custom_fields'

export const emptyContactForm = {
  firstName: '',
  lastName: '',
  email: '',
  phone: '',
  addressLine1: '',
  addressLine2: '',
  city: '',
  state: '',
  postalCode: '',
  country: '',
  jobTitle: '',
  status: 'lead',
  customFields: {}
}

export const emptyContactTaskForm = {
  title: '',
  description: '',
  dueAt: '',
  assignedToUserId: ''
}

export const emptyCallForm = {
  disposition: '',
  notes: ''
}

export const emptyManualCallForm = {
  phoneNumber: '',
  disposition: '',
  notes: ''
}

export const emptyRecordingForm = {
  recordingUrl: '',
  recordingConsent: 'unknown',
  retentionDays: '365'
}

export const emptySMSForm = {
  templateName: '',
  body: ''
}

export const emptyInboundSMSForm = {
  body: ''
}

export function defaultMeetingTimezone() {
  return Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
}

export function emptyMeetingForm() {
  return {
    title: '',
    description: '',
    location: '',
    startAt: '',
    endAt: '',
    timezone: defaultMeetingTimezone(),
    visibility: 'shared'
  }
}

export function formatContactAddress(contact = {}) {
  const street = [contact.addressLine1, contact.addressLine2].filter(Boolean).join(', ')
  const locality = [contact.city, contact.state, contact.postalCode].filter(Boolean).join(', ')
  return [street, locality, contact.country].filter(Boolean).join(' | ')
}

export function fullContactName(contact) {
  return `${contact.firstName || ''} ${contact.lastName || ''}`.trim()
}

export function contactFormValues(contact, definitions = []) {
  return {
    firstName: contact.firstName || '',
    lastName: contact.lastName || '',
    email: contact.email || '',
    phone: contact.phone || '',
    addressLine1: contact.addressLine1 || '',
    addressLine2: contact.addressLine2 || '',
    city: contact.city || '',
    state: contact.state || '',
    postalCode: contact.postalCode || '',
    country: contact.country || '',
    jobTitle: contact.jobTitle || '',
    status: contact.status || 'lead',
    customFields: customFieldFormValues(definitions, contact.customFields)
  }
}

export function contactPayload(form, definitions = []) {
  return { ...form, customFields: customFieldPayload(definitions, form.customFields) }
}

export function duplicateSearchTerm(message, fallback = '') {
  const text = String(message || '')
  const marker = text.toLowerCase().lastIndexOf('duplicate contact:')
  if (marker >= 0) {
    const candidate = text.slice(marker + 'duplicate contact:'.length).split('(')[0].trim()
    if (candidate) {
      return candidate
    }
  }
  return String(fallback || '').trim()
}

export function localDateTimeToISOString(value) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '' : date.toISOString()
}

export function recordingFormValues(call) {
  return {
    recordingUrl: call?.recordingUrl || '',
    recordingConsent: call?.recordingConsent || 'unknown',
    retentionDays: '365'
  }
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

import { apiRequest, apiURL } from './api'

export async function listCRMExports({ signal } = {}) {
  const payload = await apiRequest('/api/crm-exports', { fallbackMessage: 'Unable to load CRM exports.', signal })
  return payload?.data?.exports || []
}

export async function requestCRMExport(request, idempotencyKey) {
  const payload = await apiRequest('/api/crm-exports', {
    method: 'POST', body: request, headers: { 'Idempotency-Key': idempotencyKey }, fallbackMessage: 'Unable to request the CRM export.'
  })
  return payload?.data?.export || null
}

export function crmExportDownloadURL(exportID) {
  return apiURL(`/api/crm-exports/${exportID}/download`)
}

export function crmExportSetupURL(request) {
  return `/settings/operations?crmExport=${encodeURIComponent(JSON.stringify(request))}`
}

export function crmExportOwnership(ownerFilter) {
  return {
    ownerUserId: Number(ownerFilter) || 0,
    unassigned: ownerFilter === 'unassigned'
  }
}

export function initialCRMExportRequest() {
  try {
    const value = JSON.parse(new URLSearchParams(globalThis.location?.search || '').get('crmExport') || '')
    if (['contacts', 'companies', 'deals', 'tasks'].includes(value?.resource)) return value
  } catch {
    return { resource: 'contacts', search: '' }
  }
  return { resource: 'contacts', search: '' }
}

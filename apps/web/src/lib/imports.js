import { apiRequest, apiURL } from './api'

function importForm(file, entityType, mapping, idempotencyKey = '') {
  const form = new FormData()
  form.append('file', file)
  form.append('entityType', entityType)
  if (mapping) {
    form.append('mapping', JSON.stringify(mapping))
  }
  if (idempotencyKey) {
    form.append('idempotencyKey', idempotencyKey)
  }
  return form
}

export async function previewImport(file, entityType, mapping) {
  const payload = await apiRequest('/api/imports/preview', {
    method: 'POST',
    body: importForm(file, entityType, mapping),
    fallbackMessage: 'Unable to preview CSV import.'
  })
  return payload?.data || null
}

export async function executeImport(file, entityType, mapping, idempotencyKey) {
  const payload = await apiRequest('/api/imports', {
    method: 'POST',
    body: importForm(file, entityType, mapping, idempotencyKey),
    fallbackMessage: 'Unable to import CSV data.'
  })
  return payload?.data?.batch || null
}

export async function listImports({ limit = 50, signal } = {}) {
  const payload = await apiRequest(`/api/imports?limit=${limit}`, {
    fallbackMessage: 'Unable to load import history.',
    signal
  })
  return payload?.data?.batches || []
}

export async function rollbackImport(batchId) {
  const payload = await apiRequest(`/api/imports/${batchId}/rollback`, {
    method: 'POST',
    fallbackMessage: 'Unable to roll back import.'
  })
  return payload?.data?.batch || null
}

export function importErrorsURL(batchId) {
  return apiURL(`/api/imports/${batchId}/errors.csv`)
}

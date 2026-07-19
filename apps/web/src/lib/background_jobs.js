import { apiRequest } from './api'

export async function listBackgroundJobs({ status = '', type = '', limit = 50, signal } = {}) {
  const params = new URLSearchParams()
  if (status) {
    params.set('status', status)
  }
  if (type) {
    params.set('type', type)
  }
  if (limit) {
    params.set('limit', String(limit))
  }
  const query = params.toString()
  const payload = await apiRequest(`/api/admin/background-jobs${query ? `?${query}` : ''}`, {
    fallbackMessage: 'Unable to load background jobs.',
    signal
  })
  return {
    jobs: payload?.data?.jobs || [],
    stats: payload?.data?.stats || {}
  }
}

export async function replayBackgroundJob(jobId) {
  const payload = await apiRequest(`/api/admin/background-jobs/${jobId}/replay`, {
    method: 'POST',
    fallbackMessage: 'Unable to replay background job.'
  })
  return payload?.data?.job || null
}

export async function resolveSequenceDelivery(jobId, resolution) {
  const payload = await apiRequest(`/api/admin/background-jobs/${jobId}/resolve-sequence-delivery`, {
    method: 'POST',
    body: { resolution },
    fallbackMessage: 'Unable to resolve sequence delivery.'
  })
  return payload?.data?.resolution || null
}

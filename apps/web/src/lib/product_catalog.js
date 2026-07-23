import { apiRequest } from './api'
import { loadCompleteCatalog } from './complete_catalog'

export async function listProductCatalogPage({ search = '', status = 'all', page = 1, pageSize = 50, signal } = {}) {
  const query = new URLSearchParams()
  if (search) query.set('q', search)
  if (status && status !== 'all') query.set('status', status)
  if (page) query.set('page', String(page))
  if (pageSize) query.set('pageSize', String(pageSize))
  const payload = await apiRequest(`/api/product-catalog-items?${query.toString()}`, { fallbackMessage: 'Unable to load product catalog.', signal })
  const data = payload?.data || {}

  return { items: data.items || [], meta: data.meta || { page, pageSize, total: (data.items || []).length } }
}

export async function listProductCatalogItems({ signal } = {}) {
  return loadCompleteCatalog(
    ({ page, pageSize }) => listProductCatalogPage({ status: 'active', page, pageSize, signal }),
    'items',
    'The product catalog changed while quote options were loading. Try again.',
    'The complete active product catalog could not be loaded. Archive legacy overflow and try again.'
  )
}

export async function createProductCatalogItem(input, { signal } = {}) {
  const payload = await apiRequest('/api/product-catalog-items', { method: 'POST', body: input, fallbackMessage: 'Unable to save catalog item.', signal })

  return payload?.data?.item
}

export async function updateProductCatalogItem(itemId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/product-catalog-items/${itemId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update catalog item.', signal })

  return payload?.data?.item
}

export async function archiveProductCatalogItem(itemId, { signal } = {}) {
  await apiRequest(`/api/product-catalog-items/${itemId}`, { method: 'DELETE', fallbackMessage: 'Unable to archive catalog item.', signal })
}

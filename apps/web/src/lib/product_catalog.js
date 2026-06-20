import { apiRequest } from './api'

export async function listProductCatalogItems({ signal } = {}) {
  const payload = await apiRequest('/api/product-catalog-items', { fallbackMessage: 'Unable to load product catalog.', signal })

  return payload?.data?.items || []
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

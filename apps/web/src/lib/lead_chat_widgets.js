import { apiRequest } from './api'

export async function listLeadChatWidgets({ signal } = {}) {
  const payload = await apiRequest('/api/lead-chat-widgets', { fallbackMessage: 'Unable to load lead chat widgets.', signal })

  return payload?.data?.widgets || []
}

export async function createLeadChatWidget(input, { signal } = {}) {
  const payload = await apiRequest('/api/lead-chat-widgets', { method: 'POST', body: input, fallbackMessage: 'Unable to save lead chat widget.', signal })

  return payload?.data?.widget
}

export async function updateLeadChatWidget(widgetId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/lead-chat-widgets/${widgetId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update lead chat widget.', signal })

  return payload?.data?.widget
}

export async function getPublicLeadChatWidget(publicId, { signal } = {}) {
  const payload = await apiRequest(`/api/public/lead-chat-widgets/${encodeURIComponent(publicId)}`, { fallbackMessage: 'Unable to load lead chat widget.', signal })

  return payload?.data
}

export function publicLeadChatWidgetURL(publicId) {
  if (typeof window === 'undefined') {
    return `/widget/${publicId || ''}`
  }
  return `${window.location.origin}/widget/${publicId || ''}`
}

export function leadChatWidgetEmbedCode(widget) {
  const src = publicLeadChatWidgetURL(widget?.publicId || '')
  const position = widget?.position || 'bottom-right'
  const fixedStyle = position === 'bottom-left'
    ? 'position:fixed;left:24px;bottom:24px;z-index:9999;'
    : 'position:fixed;right:24px;bottom:24px;z-index:9999;'
  const style = position === 'inline'
    ? 'border:0;width:360px;max-width:100%;height:620px;'
    : `border:0;width:360px;max-width:calc(100vw - 32px);height:620px;${fixedStyle}`

  return `<iframe src="${src}" title="${widget?.title || 'Lead chat widget'}" style="${style}"></iframe>`
}

import { apiRequest } from './api'
import { loadCompleteCatalog } from './complete_catalog'

export async function listEmailTemplatePage({ search = '', page = 1, pageSize = 50, signal } = {}) {
  const query = definitionQuery(search, page, pageSize)
  const payload = await apiRequest(`/api/email-templates?${query}`, { fallbackMessage: 'Unable to load email templates.', signal })
  const data = payload?.data || {}
  const templates = data.templates || []
  return { templates, meta: data.meta || { page, pageSize, total: templates.length } }
}

export async function listEmailTemplates({ signal } = {}) {
  return loadCompleteDefinitions('template', ({ page, pageSize }) => listEmailTemplatePage({ page, pageSize, signal }))
}

export async function listEmailTemplateMergeFields({ signal } = {}) {
  const payload = await apiRequest('/api/email-templates/merge-fields', { fallbackMessage: 'Unable to load merge fields.', signal })

  return payload?.data?.groups || []
}

export async function listEmailSnippetPage({ search = '', page = 1, pageSize = 50, signal } = {}) {
  const query = definitionQuery(search, page, pageSize)
  const payload = await apiRequest(`/api/email-snippets?${query}`, { fallbackMessage: 'Unable to load email snippets.', signal })
  const data = payload?.data || {}
  const snippets = data.snippets || []
  return { snippets, meta: data.meta || { page, pageSize, total: snippets.length } }
}

export async function listEmailSnippets({ signal } = {}) {
  return loadCompleteDefinitions('snippet', ({ page, pageSize }) => listEmailSnippetPage({ page, pageSize, signal }))
}

export async function createEmailTemplate(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-templates', { method: 'POST', body: input, fallbackMessage: 'Unable to save email template.', signal })

  return payload?.data?.template
}

export async function updateEmailTemplate(templateId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-templates/${templateId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update email template.', signal })

  return payload?.data?.template
}

export async function deleteEmailTemplate(templateId, revision, { signal } = {}) {
  await apiRequest(`/api/email-templates/${templateId}?revision=${encodeURIComponent(revision)}`, { method: 'DELETE', fallbackMessage: 'Unable to delete email template.', signal })
}

export async function createEmailSnippet(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-snippets', { method: 'POST', body: input, fallbackMessage: 'Unable to save email snippet.', signal })

  return payload?.data?.snippet
}

export async function updateEmailSnippet(snippetId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-snippets/${snippetId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update email snippet.', signal })

  return payload?.data?.snippet
}

export async function deleteEmailSnippet(snippetId, revision, { signal } = {}) {
  await apiRequest(`/api/email-snippets/${snippetId}?revision=${encodeURIComponent(revision)}`, { method: 'DELETE', fallbackMessage: 'Unable to delete email snippet.', signal })
}

function definitionQuery(search, page, pageSize) {
  const query = new URLSearchParams()
  if (search) query.set('q', search)
  query.set('page', String(page))
  query.set('pageSize', String(pageSize))
  return query.toString()
}

async function loadCompleteDefinitions(kind, loadPage) {
  const field = kind === 'template' ? 'templates' : 'snippets'
  return loadCompleteCatalog(
    loadPage,
    field,
    `The email ${kind} catalog changed while options were loading. Try again.`,
    `The complete email ${kind} catalog could not be loaded. Delete legacy overflow and try again.`
  )
}

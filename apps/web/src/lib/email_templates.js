import { apiRequest } from './api'

export async function listEmailTemplates({ signal } = {}) {
  const payload = await apiRequest('/api/email-templates', { fallbackMessage: 'Unable to load email templates.', signal })

  return payload?.data?.templates || []
}

export async function listEmailTemplateMergeFields({ signal } = {}) {
  const payload = await apiRequest('/api/email-templates/merge-fields', { fallbackMessage: 'Unable to load merge fields.', signal })

  return payload?.data?.groups || []
}

export async function listEmailSnippets({ signal } = {}) {
  const payload = await apiRequest('/api/email-snippets', { fallbackMessage: 'Unable to load email snippets.', signal })

  return payload?.data?.snippets || []
}

export async function createEmailTemplate(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-templates', { method: 'POST', body: input, fallbackMessage: 'Unable to save email template.', signal })

  return payload?.data?.template
}

export async function updateEmailTemplate(templateId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-templates/${templateId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update email template.', signal })

  return payload?.data?.template
}

export async function deleteEmailTemplate(templateId, { signal } = {}) {
  await apiRequest(`/api/email-templates/${templateId}`, { method: 'DELETE', fallbackMessage: 'Unable to delete email template.', signal })
}

export async function createEmailSnippet(input, { signal } = {}) {
  const payload = await apiRequest('/api/email-snippets', { method: 'POST', body: input, fallbackMessage: 'Unable to save email snippet.', signal })

  return payload?.data?.snippet
}

export async function updateEmailSnippet(snippetId, input, { signal } = {}) {
  const payload = await apiRequest(`/api/email-snippets/${snippetId}`, { method: 'PATCH', body: input, fallbackMessage: 'Unable to update email snippet.', signal })

  return payload?.data?.snippet
}

export async function deleteEmailSnippet(snippetId, { signal } = {}) {
  await apiRequest(`/api/email-snippets/${snippetId}`, { method: 'DELETE', fallbackMessage: 'Unable to delete email snippet.', signal })
}

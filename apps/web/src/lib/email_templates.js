import { apiRequest } from './api'

export async function listEmailTemplates({ signal } = {}) {
  const payload = await apiRequest('/api/email-templates', { fallbackMessage: 'Unable to load email templates.', signal })

  return payload?.data?.templates || []
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

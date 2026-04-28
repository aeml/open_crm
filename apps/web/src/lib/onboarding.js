import { apiRequest } from './api'

export async function bootstrapWorkspace(input, { signal } = {}) {
  const payload = await apiRequest('/auth/bootstrap', { method: 'POST', body: input, fallbackMessage: 'Unable to create workspace.', signal })

  return payload.data
}

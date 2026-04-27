import { apiRequest } from './api'

export async function bootstrapWorkspace(input) {
  const payload = await apiRequest('/auth/bootstrap', { method: 'POST', body: input, fallbackMessage: 'Unable to create workspace.' })

  return payload.data
}

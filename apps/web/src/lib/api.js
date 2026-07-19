import { API_BASE_URL } from './config'

export class APIError extends Error {
  constructor(message, { status = 0, payload = null } = {}) {
    super(message)
    this.name = 'APIError'
    this.status = status
    this.payload = payload
  }
}

export function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

export function isAbortError(error) {
  return error?.name === 'AbortError'
}

export async function readJSON(response) {
  if (!response || typeof response.json !== 'function' || response.status === 204) {
    return {}
  }

  return response.json()
}

export async function apiRequest(path, { method = 'GET', body, headers = {}, fallbackMessage = 'Request failed.', signal } = {}) {
  const requestHeaders = { ...headers }
  const request = {
    method,
    credentials: 'include',
    headers: requestHeaders,
    signal
  }

  if (body !== undefined) {
    if (typeof FormData !== 'undefined' && body instanceof FormData) {
      request.body = body
    } else {
      requestHeaders['Content-Type'] = requestHeaders['Content-Type'] || 'application/json'
      request.body = JSON.stringify(body)
    }
  }

  const response = await fetch(`${API_BASE_URL}${path}`, request)
  const payload = await readJSON(response)

  if (!response.ok) {
    if (response.status === 401 && typeof window !== 'undefined') {
      window.dispatchEvent(new CustomEvent('auth:unauthorized'))
    }

    throw new APIError(getErrorMessage(payload, fallbackMessage), {
      status: response.status,
      payload
    })
  }

  return payload
}

export function apiURL(path) {
  return `${API_BASE_URL}${path}`
}

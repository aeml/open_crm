import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

function getCompanySaveError(response, payload, fallbackMessage) {
  const message = getErrorMessage(payload, fallbackMessage)
  if (response.status === 409) {
    return `Possible duplicate company. Review the existing record before saving again. ${message}`
  }
  return message
}

async function readJSON(response) {
  if (response.status === 204) {
    return {}
  }
  return response.json()
}

export async function listCompanies(search = '') {
  const suffix = search ? `?q=${encodeURIComponent(search)}` : ''
  const response = await fetch(`${API_BASE_URL}/api/companies${suffix}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load companies.'))
  }

  return payload?.data || { companies: [], meta: { page: 1, pageSize: 20, total: 0 } }
}

export async function getCompany(companyID) {
  const response = await fetch(`${API_BASE_URL}/api/companies/${companyID}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load company.'))
  }

  return payload?.data
}

export async function createCompany(input) {
  const response = await fetch(`${API_BASE_URL}/api/companies`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getCompanySaveError(response, payload, 'Unable to create company.'))
  }

  return payload?.data
}

export async function updateCompany(companyID, input) {
  const response = await fetch(`${API_BASE_URL}/api/companies/${companyID}`, {
    method: 'PATCH',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getCompanySaveError(response, payload, 'Unable to update company.'))
  }

  return payload?.data
}

export async function archiveCompany(companyID) {
  const response = await fetch(`${API_BASE_URL}/api/companies/${companyID}`, {
    method: 'DELETE',
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to archive company.'))
  }

  return payload
}

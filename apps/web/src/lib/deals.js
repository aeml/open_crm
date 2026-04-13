import { API_BASE_URL } from './config'

function getErrorMessage(payload, fallbackMessage) {
  return payload?.error?.message || fallbackMessage
}

async function readJSON(response) {
  if (response.status === 204) {
    return {}
  }
  return response.json()
}

export async function listDealStages() {
  const response = await fetch(`${API_BASE_URL}/api/deal-stages`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load deal stages.'))
  }

  return payload?.data?.stages || []
}

export async function listDeals(query = {}) {
  const params = new URLSearchParams()
  if (query.search) params.set('q', query.search)
  if (query.stageId) params.set('stageId', String(query.stageId))
  if (query.ownerUserId) params.set('ownerUserId', String(query.ownerUserId))
  const suffix = params.toString() ? `?${params.toString()}` : ''

  const response = await fetch(`${API_BASE_URL}/api/deals${suffix}`, {
    credentials: 'include'
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to load deals.'))
  }

  return payload?.data || { deals: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' } }
}

export async function createDeal(input) {
  const response = await fetch(`${API_BASE_URL}/api/deals`, {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(input)
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to create deal.'))
  }

	return payload?.data
}

export async function getDeal(dealID) {
	const response = await fetch(`${API_BASE_URL}/api/deals/${dealID}`, {
		credentials: 'include'
	})
	const payload = await readJSON(response)

	if (!response.ok) {
		throw new Error(getErrorMessage(payload, 'Unable to load deal.'))
	}

	return payload?.data
}

export async function updateDeal(dealID, input) {
	const response = await fetch(`${API_BASE_URL}/api/deals/${dealID}`, {
		method: 'PATCH',
		credentials: 'include',
		headers: {
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(input)
	})
	const payload = await readJSON(response)

	if (!response.ok) {
		throw new Error(getErrorMessage(payload, 'Unable to update deal.'))
	}

	return payload?.data
}

export async function archiveDeal(dealID) {
	const response = await fetch(`${API_BASE_URL}/api/deals/${dealID}`, {
		method: 'DELETE',
		credentials: 'include'
	})
	const payload = await readJSON(response)

	if (!response.ok) {
		throw new Error(getErrorMessage(payload, 'Unable to archive deal.'))
	}

	return payload
}

export async function updateDealStage(dealID, stageId) {
  const response = await fetch(`${API_BASE_URL}/api/deals/${dealID}/stage`, {
    method: 'PATCH',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ stageId })
  })
  const payload = await readJSON(response)

  if (!response.ok) {
    throw new Error(getErrorMessage(payload, 'Unable to move deal.'))
  }

  return payload?.data
}

import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => vi.unstubAllGlobals())

describe('custom fields record flow', () => {
  it('shows list values and preserves a typed filter in URL, export, and saved views', async () => {
    const definition = { id: 9, entityType: 'company', fieldKey: 'service_tier', label: 'Service tier', dataType: 'select', options: ['Gold', 'Silver'], required: false, showInList: true, position: 1 }
    const allCompanies = [
      { id: 5, name: 'Northstar Gold', clientType: 'organization', status: 'prospect', customFields: { service_tier: 'Gold' } },
      { id: 6, name: 'Southstar Silver', clientType: 'organization', status: 'prospect', customFields: { service_tier: 'Silver' } }
    ]
    const jsonResponse = (payload, status = 200) => ({ ok: status >= 200 && status < 300, status, json: async () => payload })
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'
      if (requestURL.pathname.endsWith('/auth/me')) return jsonResponse({ data: { user: { id: 1, email: 'owner@example.test' }, organization: { id: 42, name: 'Acme' }, membership: { role: 'owner' } } })
      if (requestURL.pathname.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (requestURL.pathname.endsWith('/api/custom-fields')) return jsonResponse({ data: { definitions: requestURL.searchParams.get('entityType') === 'company' ? [definition] : [] } })
      if (requestURL.pathname.endsWith('/api/companies')) {
        const filtered = requestURL.searchParams.get('customField') ? allCompanies.slice(0, 1) : allCompanies
        return jsonResponse({ data: { companies: filtered, meta: { page: 1, pageSize: 20, total: filtered.length } } })
      }
      if (requestURL.pathname.endsWith('/api/contacts')) return jsonResponse({ data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      if (requestURL.pathname.endsWith('/api/users')) return jsonResponse({ data: { users: [{ id: 1, email: 'owner@example.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      if (requestURL.pathname.endsWith('/api/saved-views') && method === 'POST') return jsonResponse({ data: { view: { id: 12, name: 'Gold accounts', entityType: 'companies', filters: JSON.parse(options.body).filters } } }, 201)
      if (requestURL.pathname.endsWith('/api/saved-views')) return jsonResponse({ data: { views: [{ id: 12, name: 'Gold accounts', entityType: 'companies', filters: { customField: 'service_tier', customOperator: 'eq', customValue: 'Gold' } }] } })
      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/companies')
    render(<AppRouter />)

    const tierLabels = await screen.findAllByText('Service tier:')
    expect(tierLabels.some((label) => label.closest('p')?.textContent.includes('Gold'))).toBe(true)
    fireEvent.change(screen.getByLabelText('Custom field filter'), { target: { value: 'service_tier' } })
    fireEvent.change(screen.getByLabelText('Value'), { target: { value: 'Gold' } })
    fireEvent.click(screen.getByRole('button', { name: 'Apply custom filter' }))

    await waitFor(() => expect(window.location.search).toContain('customField=service_tier'))
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/companies\?customField=service_tier&customOperator=eq&customValue=Gold/), expect.any(Object)))
    expect(screen.getByText('Northstar Gold')).toBeInTheDocument()
    expect(screen.queryByText('Southstar Silver')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Export CSV' })).toHaveAttribute('href', expect.stringMatching(/customField=service_tier&customOperator=eq&customValue=Gold/))
	const durableRequest = JSON.parse(new URL(screen.getByRole('link', { name: 'Queue large CSV' }).href).searchParams.get('crmExport'))
	expect(durableRequest).toMatchObject({ resource: 'companies', customField: { fieldKey: 'service_tier', operator: 'eq', value: 'Gold' } })

    fireEvent.change(screen.getByLabelText('Save current filters as'), { target: { value: 'Gold accounts' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save view' }))
    expect(await screen.findByText('Saved Gold accounts.')).toBeInTheDocument()
    const saveCall = fetchMock.mock.calls.find(([url, options]) => String(url).endsWith('/api/saved-views') && options?.method === 'POST')
    expect(JSON.parse(saveCall[1].body).filters).toMatchObject({ customField: 'service_tier', customOperator: 'eq', customValue: 'Gold' })
  })
})

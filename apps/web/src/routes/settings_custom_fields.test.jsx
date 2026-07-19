import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => vi.unstubAllGlobals())

const definition = {
  id: 9,
  entityType: 'company',
  fieldKey: 'service_tier',
  label: 'Service tier',
  dataType: 'select',
  options: ['Gold', 'Silver'],
  required: false,
  showInList: true,
  position: 2
}

describe('settings custom fields route', () => {
  it('creates, updates, and explicitly archives a stable typed definition', async () => {
    const updated = { ...definition, label: 'Account tier', required: true }
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 1 }, organization: { id: 42, name: 'Acme' }, membership: { role: 'owner' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definitions: [] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definitions: [] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definition } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definitions: [definition] } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definition: updated } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definitions: [updated] } }) })
      .mockResolvedValueOnce({ ok: true, status: 204, json: async () => ({}) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { definitions: [] } }) })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('confirm', vi.fn(() => true))
    window.history.pushState({}, '', '/settings/custom-fields')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Custom fields' })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Record type'), { target: { value: 'company' } })
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(4))
    fireEvent.change(screen.getByLabelText('Label'), { target: { value: 'Service tier' } })
    fireEvent.change(screen.getByLabelText('Type'), { target: { value: 'select' } })
    fireEvent.change(await screen.findByLabelText(/options/i), { target: { value: 'Gold, Silver' } })
    fireEvent.click(screen.getByLabelText(/show in record lists/i))
    fireEvent.click(screen.getByRole('button', { name: 'Create field' }))

    expect(await screen.findByText(/stable key custom:service_tier/i)).toBeInTheDocument()
    const createCall = fetchMock.mock.calls.find(([url, options]) => String(url).endsWith('/api/custom-fields') && options?.method === 'POST')
    expect(JSON.parse(createCall[1].body)).toMatchObject({ entityType: 'company', label: 'Service tier', dataType: 'select', options: ['Gold', 'Silver'], showInList: true })

    fireEvent.change(screen.getByLabelText('Label for service_tier'), { target: { value: 'Account tier' } })
    fireEvent.click(screen.getByLabelText('Required'))
    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }))
    expect(await screen.findByText('Account tier updated.')).toBeInTheDocument()
    const updateCall = fetchMock.mock.calls.find(([url, options]) => String(url).endsWith('/api/custom-fields/9') && options?.method === 'PATCH')
    expect(JSON.parse(updateCall[1].body)).toMatchObject({ label: 'Account tier', required: true })

    fireEvent.click(screen.getByRole('button', { name: 'Archive field' }))
    expect(await screen.findByText(/existing record values were retained/i)).toBeInTheDocument()
    expect(vi.mocked(globalThis.confirm)).toHaveBeenCalledWith(expect.stringMatching(/existing values remain stored/i))
    expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/custom-fields\/9$/), expect.objectContaining({ method: 'DELETE' }))
  })

  it('does not load management data for a member', async () => {
    const fetchMock = vi.fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { user: { id: 2 }, organization: { id: 42, name: 'Acme' }, membership: { role: 'member' } } }) })
      .mockResolvedValueOnce({ ok: true, json: async () => ({ data: { unreadCount: 0 } }) })
    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/custom-fields')
    render(<AppRouter />)
    expect(await screen.findByText(/admin access required/i)).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  })
})

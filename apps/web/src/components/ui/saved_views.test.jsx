import { StrictMode } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { SavedViews } from './saved_views'

afterEach(() => vi.unstubAllGlobals())

function response(data) {
  return { ok: true, status: 200, json: async () => ({ data }) }
}

function renderSavedViews(props = {}) {
  return render(<SavedViews
    entityType="contacts"
    currentFilters={{ status: 'lead' }}
    onApply={vi.fn()}
    {...props}
  />)
}

describe('SavedViews', () => {
  it('completes catalog loads under the application StrictMode lifecycle', async () => {
    const fetchMock = vi.fn(async () => response({
      views: [{ id: 1, entityType: 'contacts', name: 'Strict pilot view', filters: {}, isDefault: false, revision: 1 }],
      meta: { page: 1, pageSize: 100, total: 1 }
    }))
    vi.stubGlobal('fetch', fetchMock)
    render(<StrictMode>
      <SavedViews entityType="contacts" currentFilters={{}} onApply={vi.fn()} />
    </StrictMode>)

    fireEvent.click(screen.getByRole('button', { name: 'Load views' }))

    expect(await screen.findByRole('option', { name: 'Strict pilot view' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Load views' })).toBeEnabled()
    expect(screen.getByText('Saved views loaded.')).toBeInTheDocument()
  })

  it('loads every legacy page and keeps row 101 manageable', async () => {
    const views = Array.from({ length: 101 }, (_, index) => ({
      id: index + 1,
      entityType: 'contacts',
      name: `Pilot view ${String(index + 1).padStart(3, '0')}`,
      filters: {},
      isDefault: false,
      revision: 1
    }))
    const fetchMock = vi.fn(async (url) => {
      const page = Number(new URL(String(url), 'http://localhost').searchParams.get('page'))
      return response({ views: views.slice((page - 1) * 100, page * 100), meta: { page, pageSize: 100, total: views.length } })
    })
    vi.stubGlobal('fetch', fetchMock)
    renderSavedViews()

    fireEvent.click(screen.getByRole('button', { name: 'Load views' }))

    expect(await screen.findByRole('option', { name: 'Pilot view 101' })).toBeInTheDocument()
    expect(screen.getByText(/101 of 100 saved views used/)).toHaveTextContent('legacy overflow remains available to manage')
    expect(screen.getByRole('button', { name: 'Save view' })).toBeDisabled()
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('uses the selected exact revision for update and delete', async () => {
    let currentView = { id: 9, entityType: 'contacts', name: 'Reviewed leads', filters: { status: 'lead' }, isDefault: false, revision: 3 }
    const fetchMock = vi.fn(async (url, options) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (options.method === 'PATCH') {
        const input = JSON.parse(options.body)
        expect(input.expectedRevision).toBe(3)
        currentView = { ...currentView, filters: input.filters, revision: 4 }
        return response({ view: currentView })
      }
      if (options.method === 'DELETE') {
        expect(requestURL.searchParams.get('revision')).toBe('4')
        currentView = null
        return { ok: true, status: 204 }
      }
      return response({ views: currentView ? [currentView] : [], meta: { page: 1, pageSize: 100, total: currentView ? 1 : 0 } })
    })
    vi.stubGlobal('fetch', fetchMock)
    renderSavedViews({ currentFilters: { status: 'customer' } })

    fireEvent.click(screen.getByRole('button', { name: 'Load views' }))
    await screen.findByRole('option', { name: 'Reviewed leads' })
    fireEvent.change(screen.getByLabelText('Saved views'), { target: { value: '9' } })
    fireEvent.click(screen.getByRole('button', { name: 'Update' }))
    expect(await screen.findByText('Updated Reviewed leads.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }))
    expect(await screen.findByText('Deleted Reviewed leads.')).toBeInTheDocument()
    expect(fetchMock.mock.calls.filter((call) => call[1].method === 'PATCH')).toHaveLength(1)
    expect(fetchMock.mock.calls.filter((call) => call[1].method === 'DELETE')).toHaveLength(1)
  })

  it('serializes mutations and preserves successful-write status when reconciliation fails', async () => {
    let releaseCreate
    const createResponse = new Promise((resolve) => { releaseCreate = resolve })
    let getCount = 0
    const fetchMock = vi.fn(async (_url, options) => {
      if (options.method === 'POST') return createResponse
      getCount += 1
      return { ok: false, status: 503, json: async () => ({ error: { message: 'Catalog temporarily unavailable' } }) }
    })
    vi.stubGlobal('fetch', fetchMock)
    renderSavedViews()

    fireEvent.change(screen.getByLabelText('Save current filters as'), { target: { value: 'Pilot' } })
    const saveButton = screen.getByRole('button', { name: 'Save view' })
    fireEvent.click(saveButton)
    fireEvent.click(saveButton)
    expect(fetchMock.mock.calls.filter((call) => call[1].method === 'POST')).toHaveLength(1)
    releaseCreate(response({ view: { id: 12, entityType: 'contacts', name: 'Pilot', filters: { status: 'lead' }, isDefault: false, revision: 1 } }))

    expect(await screen.findByText(/Saved Pilot\. Reload failed: Catalog temporarily unavailable/)).toBeInTheDocument()
    expect(getCount).toBe(1)
    expect(screen.getByRole('button', { name: 'Save view' })).toBeEnabled()
  })

  it('discards an obsolete load after the record type changes', async () => {
    let releaseContacts
    const contactsResponse = new Promise((resolve) => { releaseContacts = resolve })
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      if (requestURL.searchParams.get('entityType') === 'contacts') return contactsResponse
      return response({ views: [{ id: 2, entityType: 'deals', name: 'Current deals', filters: {}, revision: 1 }], meta: { page: 1, pageSize: 100, total: 1 } })
    })
    vi.stubGlobal('fetch', fetchMock)
    const rendered = renderSavedViews()

    fireEvent.click(screen.getByRole('button', { name: 'Load views' }))
    rendered.rerender(<SavedViews entityType="deals" currentFilters={{}} onApply={vi.fn()} />)
    releaseContacts(response({ views: [{ id: 1, entityType: 'contacts', name: 'Obsolete contacts', filters: {}, revision: 1 }], meta: { page: 1, pageSize: 100, total: 1 } }))
    fireEvent.click(screen.getByRole('button', { name: 'Load views' }))

    expect(await screen.findByRole('option', { name: 'Current deals' })).toBeInTheDocument()
    await waitFor(() => expect(screen.queryByRole('option', { name: 'Obsolete contacts' })).not.toBeInTheDocument())
  })
})

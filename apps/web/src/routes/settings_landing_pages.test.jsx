import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload, status = 200) {
  return { ok: status < 400, status, json: async () => payload }
}

function sessionResponse() {
  return jsonResponse({
    data: {
      user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
      organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
      membership: { role: 'owner' }
    }
  })
}

const leadForm = {
  id: 3,
  name: 'Website Leads',
  slug: 'website-leads',
  publicId: 'lf_existing',
  title: 'Talk to sales',
  description: 'Tell us about your project.',
  successMessage: 'Thanks!',
  sourceLabel: 'Website form',
  isActive: true,
  submissionCount: 0,
  fields: [
    { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
    { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
    { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' }
  ]
}

describe('settings landing pages route', () => {
  it('preserves an in-progress draft when active lead forms finish loading late', async () => {
    let resolveLeadForms
    const leadFormsResponse = new Promise((resolve) => {
      resolveLeadForms = resolve
    })
    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-capture-forms')) return leadFormsResponse
      if (path.endsWith('/api/lead-landing-pages')) return jsonResponse({ data: { pages: [], meta: { page: 1, pageSize: 50, total: 0 } } })
      throw new Error(`Unexpected fetch: ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/landing-pages')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'New landing page' })).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Typed before forms loaded' } })
    resolveLeadForms(jsonResponse({ data: { forms: [leadForm], meta: { page: 1, pageSize: 100, total: 1 } } }))

    await waitFor(() => expect(screen.getByLabelText(/^lead form$/i)).toHaveValue('3'))
    expect(screen.getByLabelText(/^name$/i)).toHaveValue('Typed before forms loaded')
  })

  it('lists landing pages and creates a hosted page tied to a lead form', async () => {
	let pages = [{ id: 4, name: 'Website Demo', slug: 'website-demo', publicId: 'lp_existing', title: 'See Open CRM', subtitle: 'A focused demo page.', body: 'Learn more.', ctaLabel: 'Book demo', theme: 'light', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_existing', isActive: true, revision: 1 }]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [leadForm] } })
      if (path.endsWith('/api/lead-landing-pages') && method === 'POST') {
		const created = { id: 7, name: 'Demo Page', slug: 'demo-request', publicId: 'lp_created', title: 'Book a demo', subtitle: 'See it live', body: 'Talk to our team.', ctaLabel: 'Request demo', theme: 'blue', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_existing', isActive: true, revision: 1 }
		pages = [created, ...pages]
		return jsonResponse({ data: { page: created } }, 201)
      }
      if (path.endsWith('/api/lead-landing-pages')) {
		return jsonResponse({ data: { pages, meta: { page: 1, pageSize: 50, total: pages.length } } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/landing-pages')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /landing pages/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website demo/i })).toBeInTheDocument()
    expect(screen.getByText(/\/lp\/website-demo/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^lead form$/i), { target: { value: '3' } })
    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Demo Page' } })
    fireEvent.change(screen.getByLabelText(/slug/i), { target: { value: 'demo-request' } })
    fireEvent.change(screen.getByLabelText(/^title$/i), { target: { value: 'Book a demo' } })
    fireEvent.change(screen.getByLabelText(/^subtitle$/i), { target: { value: 'See it live' } })
    fireEvent.change(screen.getByLabelText(/^body$/i), { target: { value: 'Talk to our team.' } })
    fireEvent.change(screen.getByLabelText(/^cta label$/i), { target: { value: 'Request demo' } })
    fireEvent.change(screen.getByLabelText(/^theme$/i), { target: { value: 'blue' } })
    fireEvent.click(screen.getByRole('button', { name: /create landing page/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-landing-pages') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      expect(JSON.parse(createCall[1].body)).toEqual({
        name: 'Demo Page',
        slug: 'demo-request',
        title: 'Book a demo',
        subtitle: 'See it live',
        body: 'Talk to our team.',
        ctaLabel: 'Request demo',
        theme: 'blue',
        leadCaptureFormId: 3,
        isActive: true
      })
    })
    expect(await screen.findByRole('heading', { name: /demo page/i })).toBeInTheDocument()
  })

  it('sends the exact loaded revision when editing a landing page', async () => {
    let page = { id: 4, name: 'Revision page', slug: 'revision-page', publicId: 'lp_revision', title: 'Before edit', subtitle: '', body: '', ctaLabel: 'Submit', theme: 'light', leadCaptureFormId: 3, leadCaptureFormName: 'Website Leads', leadCaptureFormPublicId: 'lf_existing', isActive: true, revision: 7 }
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'
      if (path.endsWith('/auth/me')) return sessionResponse()
      if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
      if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [leadForm], meta: { page: 1, pageSize: 100, total: 1 } } })
      if (path.endsWith('/api/lead-landing-pages/4') && method === 'PATCH') {
        const input = JSON.parse(options.body)
        page = { ...page, ...input, revision: 8 }
        return jsonResponse({ data: { page } })
      }
      if (path.endsWith('/api/lead-landing-pages')) return jsonResponse({ data: { pages: [page], meta: { page: 1, pageSize: 50, total: 1 } } })
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/landing-pages')
    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: 'Revision page' })).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    fireEvent.change(screen.getByLabelText('Title', { exact: true }), { target: { value: 'After edit' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save landing page' }))

    await waitFor(() => {
      const updateCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-landing-pages/4') && call[1]?.method === 'PATCH'
      )
      expect(updateCall).toBeTruthy()
      expect(JSON.parse(updateCall[1].body)).toMatchObject({ title: 'After edit', revision: 7 })
    })
    expect(await screen.findByText('Landing page updated.', { exact: true })).toBeVisible()
  })

	it('loads row 51 through exact accessible continuation', async () => {
	  const pages = Array.from({ length: 51 }, (_, index) => ({
		id: index + 1,
		name: `Landing page ${String(index + 1).padStart(3, '0')}`,
		slug: `landing-page-${index + 1}`,
		publicId: `lp_${index + 1}`,
		title: `Landing page ${index + 1}`,
		ctaLabel: 'Submit',
		theme: 'light',
		leadCaptureFormId: 3,
		leadCaptureFormName: 'Website Leads',
		isActive: true,
		revision: 1
	  }))
	  const fetchMock = vi.fn(async (url) => {
		const requestURL = new URL(String(url), 'http://localhost')
		const path = requestURL.pathname
		if (path.endsWith('/auth/me')) return sessionResponse()
		if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
		if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [leadForm], meta: { page: 1, pageSize: 100, total: 1 } } })
		if (path.endsWith('/api/lead-landing-pages')) {
		  const page = Number(requestURL.searchParams.get('page'))
		  return jsonResponse({ data: { pages: pages.slice((page - 1) * 50, page * 50), meta: { page, pageSize: 50, total: pages.length } } })
		}
		throw new Error(`Unexpected fetch: ${path}`)
	  })
	  vi.stubGlobal('fetch', fetchMock)
	  window.history.pushState({}, '', '/settings/landing-pages')
	  render(<AppRouter />)

	  expect(await screen.findByText('Showing 50 of 51 landing pages.', { exact: true })).toBeVisible()
	  expect(screen.queryByRole('heading', { name: 'Landing page 051' })).not.toBeInTheDocument()
	  fireEvent.click(screen.getByRole('button', { name: 'Next landing page' }))
	  expect(await screen.findByRole('heading', { name: 'Landing page 051' })).toBeVisible()
	  expect(screen.getByText('Showing 1 of 51 landing pages.', { exact: true })).toBeVisible()
	})
})

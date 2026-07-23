import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

function jsonResponse(payload) {
  return { ok: true, json: async () => payload }
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

const standardFields = [
  { key: 'firstName', label: 'First name', fieldType: 'text', required: true, mapTo: 'firstName' },
  { key: 'lastName', label: 'Last name', fieldType: 'text', required: true, mapTo: 'lastName' },
  { key: 'email', label: 'Email', fieldType: 'email', required: true, mapTo: 'email' },
  { key: 'phone', label: 'Phone', fieldType: 'tel', required: false, mapTo: 'phone' },
  { key: 'message', label: 'How can we help?', fieldType: 'textarea', required: false, mapTo: '' }
]

const contactCustomFields = [
  { id: 19, fieldKey: 'relationship_segment', label: 'Relationship segment', dataType: 'select', required: true, options: ['Customer', 'Partner'] }
]

describe('settings lead forms route', () => {
	it('preserves in-progress input and appends required custom fields when dependencies finish late', async () => {
		let resolveCustomFields
		const customFieldsResponse = new Promise((resolve) => {
			resolveCustomFields = resolve
		})
		let createBody
		const fetchMock = vi.fn(async (url, options = {}) => {
			const requestURL = new URL(String(url), 'http://localhost')
			const path = requestURL.pathname
			const method = options.method || 'GET'
			if (path.endsWith('/auth/me')) return sessionResponse()
			if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
			if (path.endsWith('/api/custom-fields')) return customFieldsResponse
			if (path.endsWith('/api/lead-capture-submissions')) return jsonResponse({ data: { submissions: [], counts: { unreviewed: 0, legitimate: 0, spam: 0 }, limit: 50 } })
			if (path.endsWith('/api/lead-capture-forms') && method === 'POST') {
				createBody = JSON.parse(options.body)
				return jsonResponse({ data: { form: { ...createBody, id: 8, publicId: 'lf_created', revision: 1 } } })
			}
			if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [], meta: { page: 1, pageSize: 50, total: 0 } } })
			throw new Error(`Unexpected fetch: ${method} ${path}`)
		})

		vi.stubGlobal('fetch', fetchMock)
		window.history.pushState({}, '', '/settings/lead-forms')
		render(<AppRouter />)

		expect(await screen.findByRole('heading', { name: 'New lead form' })).toBeInTheDocument()
		fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Typed before dependencies' } })
		resolveCustomFields(jsonResponse({ data: { definitions: contactCustomFields } }))

		expect(await screen.findByLabelText('custom_relationship_segment destination')).toHaveValue('custom:relationship_segment')
		expect(screen.getByLabelText(/^name$/i)).toHaveValue('Typed before dependencies')
		fireEvent.click(screen.getByRole('button', { name: 'Create lead form' }))

		await waitFor(() => {
			expect(createBody?.fields.find((field) => field.key === 'custom_relationship_segment')).toEqual(expect.objectContaining({
				mapTo: 'custom:relationship_segment',
				required: true
			}))
		})
	})

	it('shows exact totals and reaches the next stable form page', async () => {
		const forms = Array.from({ length: 51 }, (_, index) => ({
			id: index + 1,
			name: `Browser lead form #${String(index + 1).padStart(3, '0')}`,
			slug: `browser-lead-form-${index + 1}`,
			publicId: `lf_browser_${index + 1}`,
			revision: 1,
			title: `Browser lead form ${index + 1}`,
			isActive: true,
			fields: standardFields
		}))
		const fetchMock = vi.fn(async (url) => {
			const requestURL = new URL(String(url), 'http://localhost')
			const path = requestURL.pathname
			if (path.endsWith('/auth/me')) return sessionResponse()
			if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
			if (path.endsWith('/api/custom-fields')) return jsonResponse({ data: { definitions: [] } })
			if (path.endsWith('/api/lead-capture-submissions')) return jsonResponse({ data: { submissions: [], counts: { unreviewed: 0, legitimate: 0, spam: 0 }, limit: 50 } })
			if (path.endsWith('/api/lead-capture-forms')) {
				const page = Number(requestURL.searchParams.get('page') || 1)
				const pageSize = Number(requestURL.searchParams.get('pageSize') || 50)
				return jsonResponse({ data: { forms: forms.slice((page - 1) * pageSize, page * pageSize), meta: { page, pageSize, total: forms.length } } })
			}
			throw new Error(`Unexpected fetch: ${path}`)
		})

		vi.stubGlobal('fetch', fetchMock)
		window.history.pushState({}, '', '/settings/lead-forms')
		render(<AppRouter />)

		expect(await screen.findByRole('heading', { name: 'Browser lead form #001' })).toBeInTheDocument()
		expect(screen.queryByRole('heading', { name: 'Browser lead form #051' })).not.toBeInTheDocument()
		expect(screen.getByText('Showing 50 of 51 lead forms.')).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Next form page' }))
		expect(await screen.findByRole('heading', { name: 'Browser lead form #051' })).toBeInTheDocument()
		expect(screen.getByText('Showing 1 of 51 lead forms.')).toBeInTheDocument()
		expect(fetchMock.mock.calls.some(([url]) => {
			const requestURL = new URL(String(url), 'http://localhost')
			return requestURL.pathname.endsWith('/api/lead-capture-forms') && requestURL.searchParams.get('page') === '2' && requestURL.searchParams.get('pageSize') === '50'
		})).toBe(true)
	})

  it('lists lead forms and creates a mapped form', async () => {
	let forms = [{ id: 3, name: 'Website Leads', slug: 'website-leads', publicId: 'lf_existing', revision: 7, title: 'Talk to sales', description: 'Main website form', successMessage: 'Thanks!', sourceLabel: 'Website form', consentText: 'I agree to receive a reply.', isActive: true, submissionCount: 2, fields: standardFields }]
    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const path = requestURL.pathname
      const method = options.method || 'GET'

      if (path.endsWith('/auth/me')) {
        return sessionResponse()
      }
      if (path.endsWith('/api/notifications/unread-count')) {
        return jsonResponse({ data: { unreadCount: 0 } })
      }
	  if (path.endsWith('/api/custom-fields')) {
		return jsonResponse({ data: { definitions: contactCustomFields } })
	  }
	  if (path.endsWith('/api/lead-capture-submissions')) {
		return jsonResponse({ data: { submissions: [], counts: { unreviewed: 0, legitimate: 0, spam: 0 }, limit: 50 } })
	  }
      if (path.endsWith('/api/lead-capture-forms') && method === 'POST') {
		const created = { id: 8, name: 'Demo request', slug: 'demo-request', publicId: 'lf_created', revision: 1, title: 'Book a demo', description: '', successMessage: 'Thanks!', sourceLabel: 'Demo form', consentText: 'I agree to be contacted about this request.', isActive: true, submissionCount: 0, fields: standardFields }
		forms = [created, ...forms]
        return jsonResponse({ data: { form: created } })
      }
      if (path.endsWith('/api/lead-capture-forms')) {
		const page = Number(requestURL.searchParams.get('page') || 1)
		const pageSize = Number(requestURL.searchParams.get('pageSize') || 50)
        return jsonResponse({ data: { forms: forms.slice((page - 1) * pageSize, page * pageSize), meta: { page, pageSize, total: forms.length } } })
      }
      throw new Error(`Unexpected fetch: ${method} ${path}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/settings/lead-forms')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /lead forms/i })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { name: /website leads/i })).toBeInTheDocument()
    expect(screen.getByDisplayValue(/api\/public\/lead-capture-forms\/lf_existing\/submissions/)).toBeInTheDocument()
	const existingEmbed = screen.getByDisplayValue(/api\/public\/lead-capture-forms\/lf_existing\/challenge/).value
	expect(existingEmbed).toContain('consentGranted')
	expect(existingEmbed).toContain('Number(challenge.formRevision) !== 7')

    fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Demo request' } })
    fireEvent.change(screen.getByLabelText(/slug/i), { target: { value: 'demo-request' } })
    fireEvent.change(screen.getByLabelText(/^title$/i), { target: { value: 'Book a demo' } })
    fireEvent.change(screen.getByLabelText(/^success message$/i), { target: { value: 'Thanks!' } })
    fireEvent.change(screen.getByLabelText(/^source label$/i), { target: { value: 'Demo form' } })
    fireEvent.click(screen.getByRole('button', { name: /create lead form/i }))

    await waitFor(() => {
      const createCall = fetchMock.mock.calls.find(
        (call) => String(call[0]).endsWith('/api/lead-capture-forms') && call[1]?.method === 'POST'
      )
      expect(createCall).toBeTruthy()
      const body = JSON.parse(createCall[1].body)
      expect(body.name).toBe('Demo request')
      expect(body.slug).toBe('demo-request')
      expect(body.consentText).toBe('I agree to be contacted about this request.')
      expect(body.fields.find((field) => field.key === 'firstName').mapTo).toBe('firstName')
      expect(body.fields.find((field) => field.key === 'message').mapTo).toBe('')
	  expect(body.fields.find((field) => field.key === 'custom_relationship_segment')).toEqual(expect.objectContaining({
		fieldType: 'select',
		mapTo: 'custom:relationship_segment',
		required: true,
		options: ['Customer', 'Partner']
	  }))
    })
    expect(await screen.findByRole('heading', { name: /demo request/i })).toBeInTheDocument()
  })

	it('sends the exact loaded revision when editing a form', async () => {
		const fetchMock = vi.fn(async (url, options = {}) => {
			const requestURL = new URL(String(url), 'http://localhost')
			const path = requestURL.pathname
			const method = options.method || 'GET'
			if (path.endsWith('/auth/me')) return sessionResponse()
			if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
			if (path.endsWith('/api/custom-fields')) return jsonResponse({ data: { definitions: [] } })
			if (path.endsWith('/api/lead-capture-submissions')) return jsonResponse({ data: { submissions: [], counts: { unreviewed: 0, legitimate: 0, spam: 0 }, limit: 50 } })
			if (path.endsWith('/api/lead-capture-forms/3') && method === 'PATCH') {
				const body = JSON.parse(options.body)
				return jsonResponse({ data: { form: { ...body, id: 3, publicId: 'lf_existing', revision: 8 } } })
			}
			if (path.endsWith('/api/lead-capture-forms')) {
				return jsonResponse({ data: { forms: [{ id: 3, name: 'Website Leads', slug: 'website-leads', publicId: 'lf_existing', revision: 7, title: 'Talk to sales', successMessage: 'Thanks!', sourceLabel: 'Website form', consentText: 'I agree to receive a reply.', isActive: true, fields: standardFields }] } })
			}
			throw new Error(`Unexpected fetch: ${method} ${path}`)
		})

		vi.stubGlobal('fetch', fetchMock)
		window.history.pushState({}, '', '/settings/lead-forms')
		render(<AppRouter />)

		expect(await screen.findByRole('heading', { name: 'Website Leads' })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
		fireEvent.change(screen.getByLabelText(/^name$/i), { target: { value: 'Website Leads revised' } })
		fireEvent.click(screen.getByRole('button', { name: 'Save lead form' }))

		await waitFor(() => {
			const updateCall = fetchMock.mock.calls.find((call) => String(call[0]).endsWith('/api/lead-capture-forms/3') && call[1]?.method === 'PATCH')
			expect(updateCall).toBeTruthy()
			const body = JSON.parse(updateCall[1].body)
			expect(body.name).toBe('Website Leads revised')
			expect(body.revision).toBe(7)
		})
		expect(await screen.findByText('Lead form updated.')).toBeInTheDocument()
	})

	it('quarantines and recovers captured spam with explicit workflow effects', async () => {
		let reviewStatus = 'unreviewed'
		const reviewCalls = []
		const submission = (status = reviewStatus) => ({
			id: 17,
			formId: 3,
			formName: 'Website Leads',
			contactId: 22,
			contactName: 'Taylor Inbound',
			contactEmail: 'taylor@example.test',
			contactActive: status !== 'spam',
			contactQuarantined: status === 'spam',
			values: { firstName: 'Taylor', message: 'Need a CRM pilot' },
			leadSource: 'Website form',
			utmSource: 'pilot',
			reviewStatus: status,
			reviewVersion: status === 'unreviewed' ? 0 : 1,
			reviewNote: '',
			queuedFollowUpRuns: status === 'unreviewed' ? 1 : 0,
			cancelledFollowUpRuns: status === 'spam' ? 1 : 0,
			completedFollowUpRuns: 0,
			createdAt: '2026-07-21T12:00:00Z'
		})
		const fetchMock = vi.fn(async (url, options = {}) => {
			const requestURL = new URL(String(url), 'http://localhost')
			const path = requestURL.pathname
			const method = options.method || 'GET'
			if (path.endsWith('/auth/me')) return sessionResponse()
			if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
			if (path.endsWith('/api/custom-fields')) return jsonResponse({ data: { definitions: [] } })
			if (path.endsWith('/api/lead-capture-forms')) {
				return jsonResponse({ data: { forms: [{ id: 3, name: 'Website Leads', publicId: 'lf_existing', isActive: true, fields: standardFields }] } })
			}
			if (path.endsWith('/api/lead-capture-submissions') && method === 'GET') {
				const requestedStatus = requestURL.searchParams.get('status') || ''
				return jsonResponse({ data: {
					submissions: !requestedStatus || requestedStatus === reviewStatus ? [submission()] : [],
					counts: { unreviewed: reviewStatus === 'unreviewed' ? 1 : 0, legitimate: reviewStatus === 'legitimate' ? 1 : 0, spam: reviewStatus === 'spam' ? 1 : 0 },
					limit: 50
				} })
			}
			if (path.endsWith('/api/lead-capture-submissions/17/review') && method === 'POST') {
				const body = JSON.parse(options.body)
				reviewCalls.push({ body, headers: options.headers })
				reviewStatus = body.status
				return jsonResponse({ data: { submission: { ...submission(), effects: body.status === 'spam' ? { cancelledRuns: 1, completedRuns: 1 } : { recoveredRuns: 1 } } } })
			}
			throw new Error(`Unexpected fetch: ${method} ${path}`)
		})

		vi.stubGlobal('fetch', fetchMock)
		vi.spyOn(window, 'confirm').mockReturnValue(true)
		window.history.pushState({}, '', '/settings/lead-forms')
		render(<AppRouter />)

		expect(await screen.findByRole('heading', { name: 'Lead submission review' })).toBeInTheDocument()
		expect(await screen.findByRole('heading', { name: 'Taylor Inbound' })).toBeInTheDocument()
		fireEvent.change(screen.getByLabelText(/^Review note for Taylor Inbound/), { target: { value: 'Obvious bot inquiry' } })
		fireEvent.click(screen.getByRole('button', { name: 'Mark spam' }))
		expect(await screen.findByText('Lead quarantined. 1 queued follow-up cancelled. 1 completed follow-up remains as history.')).toBeInTheDocument()
		expect(reviewCalls[0].body).toEqual({ status: 'spam', note: 'Obvious bot inquiry' })
		expect(reviewCalls[0].headers['Idempotency-Key']).toMatch(/^lead-review-/)

		fireEvent.change(screen.getByLabelText('Review status'), { target: { value: 'spam' } })
		expect(await screen.findByRole('button', { name: 'Recover as legitimate' })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Recover as legitimate' }))
		expect(await screen.findByText(/Lead restored as legitimate\. 1 follow-up rescheduled/)).toBeInTheDocument()
		expect(reviewCalls[1].body.status).toBe('legitimate')
	})

	it('loads older review submissions once without replacing a newer duplicate', async () => {
		const current = { id: 31, formId: 3, formName: 'Website Leads', contactId: 41, contactName: 'Newest Lead', contactEmail: 'newest@example.test', contactActive: true, values: { message: 'Newest request' }, reviewStatus: 'unreviewed', reviewVersion: 0, createdAt: '2026-07-22T12:00:00.123456Z' }
		const older = { id: 30, formId: 3, formName: 'Website Leads', contactId: 40, contactName: 'Older Lead', contactEmail: 'older@example.test', contactActive: true, values: { message: 'Older request' }, reviewStatus: 'unreviewed', reviewVersion: 0, createdAt: '2026-07-21T12:00:00.123456Z' }
		const fetchMock = vi.fn(async (url) => {
			const requestURL = new URL(String(url), 'http://localhost')
			const path = requestURL.pathname
			if (path.endsWith('/auth/me')) return sessionResponse()
			if (path.endsWith('/api/notifications/unread-count')) return jsonResponse({ data: { unreadCount: 0 } })
			if (path.endsWith('/api/custom-fields')) return jsonResponse({ data: { definitions: [] } })
			if (path.endsWith('/api/lead-capture-forms')) return jsonResponse({ data: { forms: [{ id: 3, name: 'Website Leads', publicId: 'lf_existing', isActive: true, fields: standardFields }] } })
			if (path.endsWith('/api/lead-capture-submissions')) {
				if (requestURL.searchParams.get('cursor')) {
					expect(requestURL.searchParams.get('cursor')).toBe('older-reviews')
					expect(requestURL.searchParams.get('limit')).toBe('1')
					return jsonResponse({ data: {
						submissions: [{ ...current, contactName: 'Stale duplicate', reviewVersion: 0 }, older],
						counts: { unreviewed: 2, legitimate: 0, spam: 0 },
						limit: 1,
						meta: { limit: 1, hasMore: false, nextCursor: '' }
					} })
				}
				return jsonResponse({ data: {
					submissions: [current],
					counts: { unreviewed: 2, legitimate: 0, spam: 0 },
					limit: 1,
					meta: { limit: 1, hasMore: true, nextCursor: 'older-reviews' }
				} })
			}
			throw new Error(`Unexpected fetch: ${path}`)
		})

		vi.stubGlobal('fetch', fetchMock)
		window.history.pushState({}, '', '/settings/lead-forms')
		render(<AppRouter />)

		expect(await screen.findByRole('heading', { name: 'Newest Lead' })).toBeInTheDocument()
		fireEvent.click(screen.getByRole('button', { name: 'Load older submissions' }))
		expect(await screen.findByRole('heading', { name: 'Older Lead' })).toBeInTheDocument()
		expect(screen.getAllByRole('heading', { name: 'Newest Lead' })).toHaveLength(1)
		expect(screen.queryByRole('heading', { name: 'Stale duplicate' })).not.toBeInTheDocument()
		expect(screen.queryByRole('button', { name: 'Load older submissions' })).not.toBeInTheDocument()
		expect(screen.getByText(/Showing 2 of 2 matching submissions/)).toBeInTheDocument()
	})
})

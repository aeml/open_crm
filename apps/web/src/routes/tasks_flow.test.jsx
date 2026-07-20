import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { AppRouter } from '../app/router'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('tasks flow', () => {
  it('loads tasks, creates a task, and completes it', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'
      const requestBody = options.body ? JSON.parse(String(options.body)) : null

      if (requestURL.pathname.endsWith('/auth/me')) {
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET' && requestURL.search === '?status=open') {
        return jsonResponse({
          data: {
            tasks: [
              { id: 51, entityType: 'deal', entityId: 12, entityLabel: 'Bluebird Rollout', title: 'Confirm installer arrival window', description: 'Need final arrival confirmation.', status: 'open', dueAt: '2099-04-16T09:00:00Z', completedAt: '', assignedToUserId: 1, assignedToUserName: 'Demo Owner', createdByUserId: 1, createdByUserName: 'Demo Owner' },
              { id: 52, entityType: 'deal', entityId: 12, entityLabel: 'Bluebird Rollout', title: 'Call Morgan about rollout timing', description: 'Confirm launch window.', status: 'open', dueAt: '2026-04-10T11:00:00Z', completedAt: '', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' },
              { id: 53, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'Send onboarding packet', description: 'Share intake forms.', status: 'open', dueAt: '', completedAt: '', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' },
              { id: 54, entityType: 'company', entityId: 6, entityLabel: 'Bluebird Health', title: 'Verify site access window', description: 'Need lockbox confirmation.', status: 'open', dueAt: '2099-04-18T15:00:00Z', completedAt: '', assignedToUserId: 0, assignedToUserName: '', createdByUserId: 1, createdByUserName: 'Demo Owner' }
            ],
            meta: { page: 1, pageSize: 20, total: 4, openCount: 4, completedCount: 0, overdueCount: 1, dueSoonCount: 0, upcomingCount: 2, noDueDateCount: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET' && requestURL.search === '?status=open&due=upcoming') {
        return jsonResponse({
          data: {
            tasks: [
              { id: 51, entityType: 'deal', entityId: 12, entityLabel: 'Bluebird Rollout', title: 'Confirm installer arrival window', description: 'Need final arrival confirmation.', status: 'open', dueAt: '2099-04-16T09:00:00Z', completedAt: '', assignedToUserId: 1, assignedToUserName: 'Demo Owner', createdByUserId: 1, createdByUserName: 'Demo Owner' },
              { id: 54, entityType: 'company', entityId: 6, entityLabel: 'Bluebird Health', title: 'Verify site access window', description: 'Need lockbox confirmation.', status: 'open', dueAt: '2099-04-18T15:00:00Z', completedAt: '', assignedToUserId: 0, assignedToUserName: '', createdByUserId: 1, createdByUserName: 'Demo Owner' }
            ],
            meta: { page: 1, pageSize: 20, total: 2, openCount: 2, completedCount: 0, overdueCount: 1, dueSoonCount: 0, upcomingCount: 2, noDueDateCount: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET' && requestURL.search === '?status=open&entityType=contact') {
        return jsonResponse({
          data: {
            tasks: [
              { id: 53, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'Send onboarding packet', description: 'Share intake forms.', status: 'open', dueAt: '', completedAt: '', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET' && requestURL.search === '?status=open&q=morgan') {
        return jsonResponse({
          data: {
            tasks: [
              { id: 52, entityType: 'deal', entityId: 12, entityLabel: 'Bluebird Rollout', title: 'Call Morgan about rollout timing', description: 'Confirm launch window.', status: 'open', dueAt: '2026-04-10T11:00:00Z', completedAt: '', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET' && requestURL.search === '?status=completed&q=morgan') {
        return jsonResponse({
          data: {
            tasks: [
              { id: 61, entityType: 'contact', entityId: 8, entityLabel: 'Morgan Lee', title: 'Call Morgan about renewal timing', description: 'Completed follow-up.', status: 'completed', dueAt: '2099-04-18T11:00:00Z', completedAt: '2099-04-18T12:30:00Z', assignedToUserId: 1, assignedToUserName: 'Demo Owner', createdByUserId: 1, createdByUserName: 'Demo Owner' }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 0, completedCount: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET' && requestURL.search === '?status=completed') {
        return jsonResponse({
          data: {
            tasks: [
              { id: 78, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'Collect signed agreement', description: 'Received yesterday.', status: 'completed', dueAt: '2026-04-09T10:00:00Z', completedAt: '2026-04-09T16:30:00Z', assignedToUserId: 1, assignedToUserName: 'Demo Owner', createdByUserId: 1, createdByUserName: 'Demo Owner' },
              { id: 77, entityType: 'deal', entityId: 12, entityLabel: 'Bluebird Rollout', title: 'Prepare rollout checklist', description: 'Completed and handed off.', status: 'completed', dueAt: '2026-04-16T09:00:00Z', completedAt: '2026-04-10T14:15:00Z', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' }
            ],
            meta: { page: 1, pageSize: 20, total: 2, openCount: 0, completedCount: 2 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && method === 'POST') {
        return jsonResponse({
          data: {
            task: { id: 77, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'Prepare rollout checklist', description: 'Lock owners before kickoff.', status: 'open', dueAt: '2026-04-16T09:00:00Z', completedAt: '', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' },
            activities: [{ id: 201, action: 'task.created', summary: 'Task created', createdAt: '2026-04-10T09:00:00Z' }]
          }
        }, { status: 201 })
      }

      if (requestURL.pathname.endsWith('/api/tasks/52') && method === 'PATCH') {
        return jsonResponse({
          data: {
            task: { id: 52, entityType: 'deal', entityId: 12, entityLabel: 'Bluebird Rollout', title: 'Call Morgan about rollout timing', description: 'Confirm launch window.', status: 'completed', dueAt: '2026-04-10T11:00:00Z', completedAt: '2026-04-11T09:30:00Z', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' },
            activities: [{ id: 203, action: 'task.completed', summary: 'Task completed', createdAt: '2026-04-11T09:30:00Z' }]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks/77') && method === 'PATCH') {
        const nextStatus = requestBody?.status === 'open' ? 'open' : 'completed'
        const nextAssignedToUserId = Number.parseInt(String(requestBody?.assignedToUserId || 0), 10) || 0
        const nextAssignedToUserName = nextAssignedToUserId === 1 ? 'Demo Owner' : nextAssignedToUserId === 2 ? 'Alex Admin' : ''
        const nextCompletedAt = nextStatus === 'completed' ? (requestBody?.completedAt || '2026-04-10T14:15:00Z') : ''
        const nextDescription = requestBody?.description || 'Completed and handed off.'
        return jsonResponse({
          data: {
            task: { id: 77, entityType: 'deal', entityId: 12, entityLabel: 'Bluebird Rollout', title: 'Prepare rollout checklist', description: nextDescription, status: nextStatus, dueAt: '2026-04-16T09:00:00Z', completedAt: nextCompletedAt, assignedToUserId: nextAssignedToUserId, assignedToUserName: nextAssignedToUserName, createdByUserId: 1, createdByUserName: 'Demo Owner' },
            activities: [{ id: 202, action: nextStatus === 'completed' ? 'task.completed' : 'task.reopened', summary: nextStatus === 'completed' ? 'Task completed' : 'Task reopened', createdAt: '2026-04-10T14:15:00Z' }]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks/78') && method === 'PATCH') {
        return jsonResponse({
          data: {
            task: { id: 78, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'Collect signed agreement', description: 'Received yesterday.', status: 'completed', dueAt: '2026-04-09T10:00:00Z', completedAt: '2026-04-09T16:30:00Z', assignedToUserId: 2, assignedToUserName: 'Alex Admin', createdByUserId: 1, createdByUserName: 'Demo Owner' },
            activities: [{ id: 205, action: 'task.reassigned', summary: 'Task reassigned', createdAt: '2026-04-09T16:30:00Z' }]
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks/77') && method === 'DELETE') {
        return { ok: true, status: 204, json: async () => ({}) }
      }

      if (requestURL.pathname.endsWith('/api/deals')) {
        return jsonResponse({
          data: {
            deals: [
              { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '60000.00' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/companies')) {
        return jsonResponse({
          data: {
            companies: [
              { id: 6, name: 'Bluebird Health', industry: 'Healthcare', phone: '555-0200', website: 'https://bluebird.example', status: 'prospect' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/contacts')) {
        return jsonResponse({
          data: {
            contacts: [
              { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@bluebird.example', phone: '555-0300', jobTitle: 'Operations Director', status: 'lead' }
            ],
            meta: { page: 1, pageSize: 20, total: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/users')) {
        return jsonResponse({
          data: {
            users: [
              { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' },
              { id: 2, email: 'alex@acme.test', firstName: 'Alex', lastName: 'Admin', role: 'admin' }
            ]
          }
        })
      }

      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^tasks$/i })).toBeInTheDocument()
    expect(await screen.findByText(/call morgan about rollout timing/i)).toBeInTheDocument()
    expect(screen.getAllByText(/open tasks/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/showing 4 of 4 open tasks/i)).toBeInTheDocument()

    const initialTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(initialTaskList).getAllByRole('button').map((button) => button.textContent)).toEqual([
      'Call Morgan about rollout timing',
      'Complete',
      'Confirm installer arrival window',
      'Complete',
      'Verify site access window',
      'Complete',
      'Send onboarding packet',
      'Complete'
    ])

    fireEvent.click(screen.getByRole('button', { name: /complete call morgan about rollout timing/i }))

    expect(await screen.findByText(/showing 3 of 3 open tasks/i)).toBeInTheDocument()
    expect(screen.queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/record type filter/i), { target: { value: 'contact' } })

    expect(screen.getByText(/showing 1 of 3 open tasks/i)).toBeInTheDocument()
    const contactTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(contactTaskList).getByText(/send onboarding packet/i)).toBeInTheDocument()
    expect(within(contactTaskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()
    expect(within(contactTaskList).queryByText(/confirm installer arrival window/i)).not.toBeInTheDocument()
    expect(within(contactTaskList).queryByText(/verify site access window/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/record type filter/i), { target: { value: 'all' } })

    fireEvent.click(screen.getByRole('button', { name: /my tasks/i }))

    expect(screen.getByText(/showing 1 of 3 open tasks/i)).toBeInTheDocument()
    const myTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(myTaskList).getByText(/confirm installer arrival window/i)).toBeInTheDocument()
    expect(within(myTaskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()
    expect(within(myTaskList).queryByText(/send onboarding packet/i)).not.toBeInTheDocument()
    expect(within(myTaskList).queryByText(/verify site access window/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: 'all' } })

    fireEvent.click(screen.getByRole('button', { name: /^unassigned$/i }))

    expect(screen.getByText(/showing 1 of 3 open tasks/i)).toBeInTheDocument()
    const unassignedTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(unassignedTaskList).getByText(/verify site access window/i)).toBeInTheDocument()
    expect(within(unassignedTaskList).getByText(/^unassigned$/i)).toBeInTheDocument()
    expect(within(unassignedTaskList).queryByText(/confirm installer arrival window/i)).not.toBeInTheDocument()
    expect(within(unassignedTaskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()
    expect(within(unassignedTaskList).queryByText(/send onboarding packet/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: 'all' } })

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: '1' } })

    expect(screen.getByText(/showing 1 of 3 open tasks/i)).toBeInTheDocument()
    const ownerTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(ownerTaskList).getByText(/confirm installer arrival window/i)).toBeInTheDocument()
    expect(within(ownerTaskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: 'all' } })

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText(/record type filter/i), { target: { value: 'contact' } })

    expect(screen.getByText(/showing 0 of 3 open tasks/i)).toBeInTheDocument()
    expect(screen.getByText(/no open tasks match the current filters/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/^assignee$/i), { target: { value: 'all' } })
    fireEvent.change(screen.getByLabelText(/record type filter/i), { target: { value: 'all' } })

    fireEvent.change(screen.getByLabelText(/task view/i), { target: { value: 'upcoming' } })

    expect(await screen.findByRole('heading', { name: /^upcoming tasks$/i })).toBeInTheDocument()
    const taskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(taskList).getByText(/confirm installer arrival window/i)).toBeInTheDocument()
    expect(within(taskList).getByText(/verify site access window/i)).toBeInTheDocument()
    expect(within(taskList).queryByText(/call morgan about rollout timing/i)).not.toBeInTheDocument()
    expect(screen.getByText(/showing 2 of 2 upcoming tasks/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/task view/i), { target: { value: 'all' } })

    fireEvent.change(screen.getByLabelText(/search tasks/i), { target: { value: 'morgan' } })

    expect(await screen.findByText(/showing 1 of 1 open tasks/i)).toBeInTheDocument()

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=open&q=morgan/), expect.any(Object))
    })

    fireEvent.change(screen.getByLabelText(/search tasks/i), { target: { value: '' } })

    expect(await screen.findByText(/showing 4 of 4 open tasks/i)).toBeInTheDocument()

    expect(screen.queryByLabelText(/entity id/i)).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/assigned to user id/i)).not.toBeInTheDocument()
    expect(screen.getByLabelText(/^deal$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^assigned to$/i)).toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/entity type/i), { target: { value: 'company' } })
    expect(screen.getByLabelText(/^company$/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/^deal$/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/entity type/i), { target: { value: 'contact' } })
    expect(screen.getByLabelText(/^contact$/i)).toBeInTheDocument()
    expect(screen.queryByLabelText(/^company$/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/task title/i), { target: { value: 'Prepare rollout checklist' } })
    fireEvent.change(screen.getByLabelText(/^contact$/i), { target: { value: '8' } })
    fireEvent.change(screen.getByLabelText(/description/i), { target: { value: 'Lock owners before kickoff.' } })
    fireEvent.change(screen.getByLabelText(/^assigned to$/i), { target: { value: '2' } })
    fireEvent.change(screen.getByLabelText(/due at/i), { target: { value: '2026-04-16T09:00' } })
    fireEvent.click(screen.getByRole('button', { name: /save task/i }))

    expect(await screen.findByRole('heading', { name: /prepare rollout checklist/i })).toBeInTheDocument()
    expect(screen.getByText(/task created/i)).toBeInTheDocument()
    expect(screen.getByText(/apr 10, 2026/i)).toBeInTheDocument()
    expect(window.location.pathname).toBe('/tasks/77')
    expect(screen.queryAllByLabelText(/assigned to user id/i)).toHaveLength(0)
    expect(screen.getAllByLabelText(/^assigned to$/i).length).toBeGreaterThan(0)

    fireEvent.change(screen.getAllByLabelText(/^assigned to$/i)[1], { target: { value: '1' } })
    fireEvent.change(screen.getByLabelText(/status/i), { target: { value: 'completed' } })
    fireEvent.change(screen.getByLabelText(/completed at/i), { target: { value: '2026-04-10T14:15' } })
    fireEvent.change(screen.getAllByLabelText(/description/i)[1], { target: { value: 'Completed and handed off.' } })
    fireEvent.click(screen.getByRole('button', { name: /update task/i }))

    expect(await screen.findByText(/task completed/i)).toBeInTheDocument()
    expect(screen.getAllByText(/completed and handed off/i).length).toBeGreaterThan(0)

    fireEvent.click(screen.getByRole('button', { name: /show completed/i }))

    expect(await screen.findByRole('heading', { name: /^completed tasks$/i })).toBeInTheDocument()
    expect(screen.getAllByText(/prepare rollout checklist/i).length).toBeGreaterThan(0)
    const completedTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(completedTaskList).getAllByRole('button').map((button) => button.textContent)).toEqual([
      'Prepare rollout checklist',
      'Reopen',
      'Collect signed agreement',
      'Reopen'
    ])

    fireEvent.click(screen.getByRole('button', { name: /reopen prepare rollout checklist/i }))

    expect(await screen.findByText(/showing 1 of 1 completed tasks/i)).toBeInTheDocument()
    expect(screen.getByText(/task reopened/i)).toBeInTheDocument()
    const reopenedCompletedTaskList = screen.getByRole('list', { name: /tasks list/i })
    expect(within(reopenedCompletedTaskList).queryByText(/prepare rollout checklist/i)).not.toBeInTheDocument()

    fireEvent.change(screen.getByLabelText(/assign collect signed agreement/i), { target: { value: '2' } })

    await waitFor(() => {
      expect(screen.getByLabelText(/assign collect signed agreement/i)).toHaveValue('2')
    })

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\/77$/), expect.objectContaining({ method: 'PATCH' }))
    })
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\/78$/), expect.objectContaining({ method: 'PATCH' }))
    })

    fireEvent.click(screen.getByRole('button', { name: /archive task/i }))

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\/77$/), expect.objectContaining({ method: 'DELETE' }))
    })
    expect(window.location.pathname).toBe('/tasks')
    expect(screen.queryByRole('heading', { name: /prepare rollout checklist/i })).not.toBeInTheDocument()
  })

  it('keeps the active task selected when an earlier quick action finishes late', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })
    let resolveCompletion
    const completionResponse = new Promise((resolve) => {
      resolveCompletion = resolve
    })
    const firstTask = { id: 1, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'First task', description: 'First detail', status: 'open', dueAt: '', completedAt: '', assignedToUserId: 1, assignedToUserName: 'Demo Owner' }
    const secondTask = { id: 2, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'Second task', description: 'Second detail', status: 'open', dueAt: '', completedAt: '', assignedToUserId: 1, assignedToUserName: 'Demo Owner' }

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'
      if (requestURL.pathname.endsWith('/auth/me')) {
        return jsonResponse({ data: { user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' }, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' }, membership: { role: 'owner' } } })
      }
      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET') {
        return jsonResponse({ data: { tasks: [firstTask, secondTask], meta: { page: 1, pageSize: 20, total: 2, openCount: 2, completedCount: 0 } } })
      }
      if (requestURL.pathname.endsWith('/api/tasks/1') && method === 'PATCH') {
        return completionResponse
      }
      if (requestURL.pathname.endsWith('/api/deals')) {
        return jsonResponse({ data: { deals: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' } } })
      }
      if (requestURL.pathname.endsWith('/api/companies')) {
        return jsonResponse({ data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }
      if (requestURL.pathname.endsWith('/api/contacts')) {
        return jsonResponse({ data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }
      if (requestURL.pathname.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: 'First task' }))
    expect(await screen.findByRole('heading', { name: 'First task' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Complete First task' }))
    expect(screen.getByRole('button', { name: 'Complete First task' })).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: 'Second task' }))
    expect(await screen.findByRole('heading', { name: 'Second task' })).toBeInTheDocument()
    expect(window.location.pathname).toBe('/tasks/2')

    await act(async () => {
      resolveCompletion(jsonResponse({
        data: {
          task: { ...firstTask, status: 'completed', completedAt: '2026-07-20T09:00:00Z' },
          activities: [{ id: 9, action: 'task.completed', summary: 'Task completed', createdAt: '2026-07-20T09:00:00Z' }]
        }
      }))
      await completionResponse
    })

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Second task' })).toBeInTheDocument()
      expect(window.location.pathname).toBe('/tasks/2')
    })
    expect(screen.getAllByLabelText(/task title/i).every((input) => input.value === 'Second task')).toBe(true)
    expect(screen.queryByRole('heading', { name: 'First task' })).not.toBeInTheDocument()
  })

  it('keeps a delayed task-form save from replacing a newer task visit', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })
    let resolveUpdate
    const updateResponse = new Promise((resolve) => {
      resolveUpdate = resolve
    })
    const firstTask = { id: 1, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'First task', description: 'First detail', status: 'open', dueAt: '', completedAt: '', assignedToUserId: 1, assignedToUserName: 'Demo Owner' }
    const secondTask = { id: 2, entityType: 'contact', entityId: 8, entityLabel: 'Ava Stone', title: 'Second task', description: 'Second detail', status: 'open', dueAt: '', completedAt: '', assignedToUserId: 1, assignedToUserName: 'Demo Owner' }

    const fetchMock = vi.fn(async (url, options = {}) => {
      const requestURL = new URL(String(url), 'http://localhost')
      const method = options.method || 'GET'
      if (requestURL.pathname.endsWith('/auth/me')) {
        return jsonResponse({ data: { user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' }, organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' }, membership: { role: 'owner' } } })
      }
      if (requestURL.pathname.endsWith('/api/tasks') && method === 'GET') {
        return jsonResponse({ data: { tasks: [firstTask, secondTask], meta: { page: 1, pageSize: 20, total: 2, openCount: 2, completedCount: 0 } } })
      }
      if (requestURL.pathname.endsWith('/api/tasks/1') && method === 'PATCH') return updateResponse
      if (requestURL.pathname.endsWith('/api/deals')) {
        return jsonResponse({ data: { deals: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' } } })
      }
      if (requestURL.pathname.endsWith('/api/companies')) {
        return jsonResponse({ data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }
      if (requestURL.pathname.endsWith('/api/contacts')) {
        return jsonResponse({ data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }
      if (requestURL.pathname.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      }
      throw new Error(`Unexpected fetch: ${method} ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks')
    render(<AppRouter />)

    fireEvent.click(await screen.findByRole('button', { name: 'First task' }))
    expect(await screen.findByRole('heading', { name: 'First task' })).toBeInTheDocument()
    fireEvent.change(screen.getAllByLabelText('Task title')[1], { target: { value: 'First edited' } })
    fireEvent.click(screen.getByRole('button', { name: 'Update task' }))
    fireEvent.click(screen.getByRole('button', { name: 'Second task' }))
    expect(await screen.findByRole('heading', { name: 'Second task' })).toBeInTheDocument()

    await act(async () => {
      resolveUpdate(jsonResponse({
        data: {
          task: { ...firstTask, title: 'First persisted' },
          activities: [{ id: 10, action: 'task.updated', summary: 'First task updated', createdAt: '2026-07-20T09:00:00Z' }]
        }
      }))
      await updateResponse
    })

    await waitFor(() => expect(screen.getByRole('button', { name: 'First persisted' })).toBeInTheDocument())
    expect(screen.getByRole('heading', { name: 'Second task' })).toBeInTheDocument()
    expect(window.location.pathname).toBe('/tasks/2')
    expect(screen.getAllByLabelText('Task title').every((input) => input.value === 'Second task')).toBe(true)
    expect(screen.queryByText('First task updated')).not.toBeInTheDocument()
  })

  it('uses service task wording for service businesses', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'services' },
            membership: { role: 'owner' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            tasks: [],
            meta: { page: 1, pageSize: 20, total: 0, openCount: 0, completedCount: 0 }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          data: {
            deals: [
              { id: 12, name: 'Bluebird Rollout', stageId: 2, stageName: 'Qualified', companyId: 6, companyName: 'Bluebird Health', primaryContactId: 8, primaryContactName: 'Ava Stone', status: 'open', valueAmount: '60000.00', valueCurrency: 'USD', expectedCloseDate: '2026-05-02', ownerUserId: 1 }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, wonCount: 0, pipelineValue: '60000.00' }
          }
        })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { contacts: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      })
      .mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^service tasks$/i })).toBeInTheDocument()
    expect(screen.getAllByText(/open service tasks/i).length).toBeGreaterThan(0)
    expect(screen.getByText(/completed service tasks/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/search service tasks/i)).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /new service task/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/linked record filter/i)).toBeInTheDocument()
    expect(screen.getAllByRole('option', { name: /^job$/i }).length).toBeGreaterThan(0)
    expect(screen.getAllByRole('option', { name: /^client$/i }).length).toBeGreaterThan(0)
  })

  it('hydrates task filters from the url and keeps them in sync', async () => {
    const jsonResponse = (payload, init = {}) => ({
      ok: init.ok ?? true,
      status: init.status ?? 200,
      json: async () => payload
    })

    const fetchMock = vi.fn(async (url) => {
      const requestURL = new URL(String(url), 'http://localhost')

      if (requestURL.pathname.endsWith('/auth/me')) {
        return jsonResponse({
          data: {
            user: { id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner' },
            organization: { id: 1, name: 'Acme, Inc.', slug: 'acme-inc', businessType: 'general' },
            membership: { role: 'owner' }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('status') === 'open' && requestURL.searchParams.get('q') === 'morgan' && requestURL.searchParams.get('entityType') === 'contact' && requestURL.searchParams.get('entityId') === '8') {
        return jsonResponse({
          data: {
            tasks: [
              {
                id: 52,
                entityType: 'contact',
                entityId: 8,
                entityLabel: 'Morgan Lee',
                title: 'Call Morgan about renewal timing',
                description: 'Confirm next review window.',
                status: 'open',
                dueAt: '2099-04-18T11:00:00Z',
                completedAt: '',
                assignedToUserId: 1,
                assignedToUserName: 'Demo Owner',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 1, completedCount: 0 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/tasks') && requestURL.searchParams.get('status') === 'completed' && requestURL.searchParams.get('q') === 'morgan' && requestURL.searchParams.get('entityType') === 'contact' && requestURL.searchParams.get('entityId') === '8') {
        return jsonResponse({
          data: {
            tasks: [
              {
                id: 61,
                entityType: 'contact',
                entityId: 8,
                entityLabel: 'Morgan Lee',
                title: 'Call Morgan about renewal timing',
                description: 'Completed follow-up.',
                status: 'completed',
                dueAt: '2099-04-18T11:00:00Z',
                completedAt: '2099-04-18T12:30:00Z',
                assignedToUserId: 1,
                assignedToUserName: 'Demo Owner',
                createdByUserId: 1,
                createdByUserName: 'Demo Owner'
              }
            ],
            meta: { page: 1, pageSize: 20, total: 1, openCount: 0, completedCount: 1 }
          }
        })
      }

      if (requestURL.pathname.endsWith('/api/deals')) {
        return jsonResponse({ data: { deals: [], meta: { page: 1, pageSize: 20, total: 0, openCount: 0, wonCount: 0, pipelineValue: '0' } } })
      }

      if (requestURL.pathname.endsWith('/api/companies')) {
        return jsonResponse({ data: { companies: [], meta: { page: 1, pageSize: 20, total: 0 } } })
      }

      if (requestURL.pathname.endsWith('/api/contacts')) {
        return jsonResponse({ data: { contacts: [{ id: 8, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@acme.test', phone: '555-0100', jobTitle: 'RevOps Lead', status: 'lead' }], meta: { page: 1, pageSize: 20, total: 1 } } })
      }

      if (requestURL.pathname.endsWith('/api/users')) {
        return jsonResponse({ data: { users: [{ id: 1, email: 'owner@acme.test', firstName: 'Demo', lastName: 'Owner', role: 'owner' }] } })
      }

      throw new Error(`Unexpected fetch: ${requestURL.pathname}${requestURL.search}`)
    })

    vi.stubGlobal('fetch', fetchMock)
    window.history.pushState({}, '', '/tasks?q=morgan&entityType=contact&entityId=8&assignee=1&due=upcoming')

    render(<AppRouter />)

    expect(await screen.findByRole('heading', { name: /^tasks$/i })).toBeInTheDocument()
    expect(await screen.findByText(/call morgan about renewal timing/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/search tasks/i)).toHaveValue('morgan')
    expect(screen.getByLabelText(/^assignee$/i)).toHaveValue('1')
    expect(screen.getByLabelText(/record type filter/i)).toHaveValue('contact')
    expect(screen.getByLabelText(/^record$/i)).toHaveValue('8')
    expect(screen.getByLabelText(/task view/i)).toHaveValue('upcoming')
    const initialParams = new URLSearchParams(window.location.search)
    expect(initialParams.get('q')).toBe('morgan')
    expect(initialParams.get('due')).toBe('upcoming')
    expect(initialParams.get('assignee')).toBe('1')
    expect(initialParams.get('entityType')).toBe('contact')
    expect(initialParams.get('entityId')).toBe('8')
    expect(initialParams.get('status')).toBeNull()

    fireEvent.click(screen.getByRole('button', { name: /show completed/i }))

    expect(await screen.findByRole('heading', { name: /^completed tasks$/i })).toBeInTheDocument()
    expect(screen.getByText(/completed 4\/18\/2099/i)).toBeInTheDocument()
    const completedParams = new URLSearchParams(window.location.search)
    expect(completedParams.get('q')).toBe('morgan')
    expect(completedParams.get('status')).toBe('completed')
    expect(completedParams.get('assignee')).toBe('1')
    expect(completedParams.get('entityType')).toBe('contact')
    expect(completedParams.get('entityId')).toBe('8')
    expect(completedParams.get('due')).toBeNull()

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(expect.stringMatching(/\/api\/tasks\?status=completed&entityType=contact&entityId=8&assignedToUserId=1&q=morgan$/), expect.any(Object))
    })
  })
})

import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClientReviewSchedule } from './client_review_schedule'

const users = [{ id: 7, firstName: 'Riley', lastName: 'Owner', email: 'riley@example.test' }]
const semantics = [
  'A client has at most one schedule.',
  'Recurring completion creates the next task.'
]

function jsonResponse(data, status = 200) {
  return new Response(JSON.stringify({ data }), { status, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => {
  vi.restoreAllMocks()
})

describe('client review schedule', () => {
  it('schedules a recurring ordinary task and exposes its exact execution state', async () => {
    const onChanged = vi.fn()
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (input, init = {}) => {
      const method = init.method || 'GET'
      if (method === 'GET') {
        return jsonResponse({ exists: false, entityType: 'company', entityId: 81, entityLabel: 'Acme', semantics })
      }
      if (method === 'PUT') {
        const body = JSON.parse(init.body)
        expect(body.reviewType).toBe('renewal')
        expect(body.cadenceMonths).toBe(12)
        expect(body.assignedToUserId).toBe(7)
        expect(new Date(body.nextReviewAt).toISOString()).toBe('2026-08-15T15:00:00.000Z')
        return jsonResponse({
          exists: true,
          entityType: 'company',
          entityId: 81,
          entityLabel: 'Acme',
          reviewType: 'renewal',
          reviewLabel: 'Client renewal',
          nextReviewAt: '2026-08-15T15:00:00Z',
          cadenceMonths: 12,
          cadenceLabel: 'Every 12 months',
          currentTaskId: 99,
          taskStatus: 'open',
          assignedToUserId: 7,
          assignedToUserName: 'Riley Owner',
          semantics
        })
      }
      throw new Error(`Unexpected ${method} ${input}`)
    })

    render(
      <MemoryRouter>
        <ClientReviewSchedule entityType="company" entityId={81} isClient canWrite users={users} onChanged={onChanged} />
      </MemoryRouter>
    )

    expect(await screen.findByText(/no review or renewal task is scheduled/i)).toBeInTheDocument()
    fireEvent.change(screen.getByLabelText('Follow-up type'), { target: { value: 'renewal' } })
    fireEvent.change(screen.getByLabelText('Next due time'), { target: { value: '2026-08-15T15:00' } })
    fireEvent.change(screen.getByLabelText(/^Cadence/), { target: { value: '12' } })
    fireEvent.click(screen.getByRole('button', { name: 'Schedule task' }))

    const obligation = await screen.findByRole('list', { name: 'Current client review obligation' })
    expect(within(obligation).getByText('Client renewal')).toBeInTheDocument()
    expect(within(obligation).getByText(/every 12 months/i)).toBeInTheDocument()
    expect(within(obligation).getByRole('link', { name: 'Open task' })).toHaveAttribute('href', '/tasks/99')
    expect(screen.getByText('Client renewal task scheduled.')).toBeInTheDocument()
    expect(onChanged).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  it('lets a writer clear the schedule and archives its open generated task contract', async () => {
    const onChanged = vi.fn()
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(async (_input, init = {}) => {
      const method = init.method || 'GET'
      if (method === 'DELETE') return new Response(null, { status: 204 })
      return jsonResponse({
        exists: true,
        entityType: 'contact',
        entityId: 82,
        entityLabel: 'Indy Client',
        reviewType: 'review',
        reviewLabel: 'Client review',
        nextReviewAt: '2026-08-20T12:00:00Z',
        cadenceMonths: 3,
        cadenceLabel: 'Every 3 months',
        currentTaskId: 101,
        taskStatus: 'open',
        assignedToUserId: 7,
        assignedToUserName: 'Riley Owner',
        semantics
      })
    })

    render(
      <MemoryRouter>
        <ClientReviewSchedule entityType="contact" entityId={82} isClient canWrite users={users} onChanged={onChanged} />
      </MemoryRouter>
    )
    expect(await screen.findByRole('link', { name: 'Open task' })).toHaveAttribute('href', '/tasks/101')
    fireEvent.click(screen.getByRole('button', { name: 'Clear schedule' }))
    expect(await screen.findByText(/open generated task was archived/i)).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Open task' })).not.toBeInTheDocument()
    expect(onChanged).toHaveBeenCalledTimes(1)
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2))
  })

  it('is absent for records that are not clients', () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch')
    const { container } = render(
      <MemoryRouter>
        <ClientReviewSchedule entityType="contact" entityId={82} isClient={false} canWrite users={users} />
      </MemoryRouter>
    )
    expect(container).toBeEmptyDOMElement()
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

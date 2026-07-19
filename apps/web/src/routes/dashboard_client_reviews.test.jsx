import { render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it } from 'vitest'
import { DashboardClientReviews } from './dashboard_client_reviews'

describe('dashboard client reviews', () => {
  it('shows exact obligation buckets and links both client and task', () => {
    render(
      <MemoryRouter>
        <DashboardClientReviews summary={{
          total: 3,
          overdue: 1,
          dueWithin30Days: 1,
          later: 1,
          records: [{
            entityType: 'company', entityId: 81, entityLabel: 'Acme Client', reviewLabel: 'Client renewal',
            nextReviewAt: '2026-08-15T15:00:00Z', cadenceLabel: 'Every 12 months', currentTaskId: 99,
            assignedToUserName: 'Riley Owner', isOverdue: true
          }],
          semantics: ['Due within 30 days excludes overdue obligations.']
        }} />
      </MemoryRouter>
    )
    const counts = screen.getByLabelText('Client obligation counts')
    expect(within(counts).getByText('1 overdue')).toBeInTheDocument()
    expect(within(counts).getByText('1 due within 30 days')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Acme Client' })).toHaveAttribute('href', '/companies/81')
    expect(screen.getByRole('link', { name: 'Review overdue task' })).toHaveAttribute('href', '/tasks/99')
  })

  it('has a truthful empty state', () => {
    render(<MemoryRouter><DashboardClientReviews summary={{ total: 0, records: [] }} /></MemoryRouter>)
    expect(screen.getByText(/add one from a customer record/i)).toBeInTheDocument()
  })
})

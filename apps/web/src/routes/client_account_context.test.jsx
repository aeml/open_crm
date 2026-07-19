import { fireEvent, render, screen, within } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { describe, expect, it, vi } from 'vitest'
import { ClientAccountContext } from './client_account_context'
import { DealCloseSummary } from './deal_close_review'

const labels = { singular: 'Deal', plural: 'Deals' }

describe('client account context', () => {
  it('summarizes existing post-sale records and keeps won deals out of the remaining deal list', () => {
    const onCreateDeal = vi.fn()
    const onOpenContact = vi.fn()
    const onOpenDeal = vi.fn()
    render(
      <ClientAccountContext
        canWrite
        contacts={[{ id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@example.test', relationshipTitle: 'Champion', isPrimary: true }]}
        deals={[
          { id: 11, name: 'Implementation', status: 'won', stageName: 'Won', valueAmount: '25000', valueCurrency: 'USD', closeReasonLabel: 'Best solution fit' },
          { id: 12, name: 'Renewal discovery', status: 'open', stageName: 'Discovery', valueAmount: '5000', valueCurrency: 'USD' },
          { id: 13, name: 'Earlier attempt', status: 'lost', stageName: 'Lost', valueAmount: '1000', valueCurrency: 'USD' }
        ]}
        isCustomer
        labels={labels}
        notes={[{ id: 31, body: 'Delivery starts Monday.', createdByUserName: 'Casey Closer' }]}
        onCreateDeal={onCreateDeal}
        onOpenContact={onOpenContact}
        onOpenDeal={onOpenDeal}
        tasks={[{ id: 21, title: 'Confirm kickoff team', assignedToUserName: 'Alex Admin', dueAt: '2026-07-24T12:00:00Z' }]}
      />
    )

    const summary = screen.getByLabelText('Client account summary')
    expect(within(summary).getByText('Won deals: 1')).toBeInTheDocument()
    expect(within(summary).getByText('Open account tasks: 1')).toBeInTheDocument()
    expect(within(summary).getByText('Implementation')).toBeInTheDocument()
    expect(within(summary).getByText('Confirm kickoff team')).toBeInTheDocument()
    expect(within(summary).getByText('Delivery starts Monday.')).toBeInTheDocument()
    expect(within(summary).getByText('Morgan Lee')).toBeInTheDocument()
    const otherDeals = screen.getByRole('list', { name: 'Related deals list' })
    expect(within(otherDeals).queryByText('Implementation')).not.toBeInTheDocument()
    expect(within(otherDeals).getByText('Renewal discovery')).toBeInTheDocument()
    expect(within(otherDeals).getByText('Earlier attempt')).toBeInTheDocument()

    fireEvent.click(within(summary).getByRole('button', { name: 'Implementation' }))
    fireEvent.click(within(summary).getByRole('button', { name: 'Morgan Lee' }))
    fireEvent.click(screen.getByRole('button', { name: 'Create Deal' }))
    expect(onOpenDeal).toHaveBeenCalledWith(11)
    expect(onOpenContact).toHaveBeenCalledWith(7)
    expect(onCreateDeal).toHaveBeenCalledOnce()
  })

  it('keeps the ordinary related-deal view for a prospect without a win', () => {
    render(<ClientAccountContext deals={[{ id: 12, name: 'Discovery', status: 'open' }]} labels={labels} onOpenDeal={() => {}} />)
    expect(screen.queryByLabelText('Client account summary')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Related deals' })).toBeInTheDocument()
    expect(screen.getByText('Discovery')).toBeInTheDocument()
  })
})

describe('won deal account link', () => {
  it('links a company win to its customer account', () => {
    render(
      <MemoryRouter>
        <DealCloseSummary deal={{ status: 'won', stageName: 'Won', companyId: 9, primaryContactId: 7, closeReasonLabel: 'Best solution fit' }} />
      </MemoryRouter>
    )
    expect(screen.getByRole('link', { name: 'Open customer account' })).toHaveAttribute('href', '/companies/9')
  })

  it('falls back to an individual client and omits links for losses', () => {
    const { rerender } = render(
      <MemoryRouter>
        <DealCloseSummary deal={{ status: 'won', stageName: 'Won', primaryContactId: 7, closeReasonLabel: 'Existing relationship' }} />
      </MemoryRouter>
    )
    expect(screen.getByRole('link', { name: 'Open customer account' })).toHaveAttribute('href', '/contacts/7')
    rerender(
      <MemoryRouter>
        <DealCloseSummary deal={{ status: 'lost', stageName: 'Lost', companyId: 9, closeReasonLabel: 'Competitor' }} />
      </MemoryRouter>
    )
    expect(screen.queryByRole('link', { name: 'Open customer account' })).not.toBeInTheDocument()
  })
})

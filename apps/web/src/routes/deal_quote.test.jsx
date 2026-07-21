import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { DealSignatureCard } from './deal_quote'

const signedRequest = {
  id: 41,
  quoteId: 71,
  quoteNumber: 'Q-12-V1',
  signerName: 'Ava Stone',
  signerEmail: 'ava@example.test',
  status: 'signed',
  provider: 'open_crm_native',
  quoteFileName: 'quote-q-12-v1.pdf',
  signedName: 'Ava Stone',
  consentedAt: '2026-07-21T08:00:00Z',
  signedAt: '2026-07-21T08:00:00Z'
}

const stages = [
  { id: 3, name: 'Proposal', isClosed: false, isWon: false },
  { id: 5, name: 'Closed Won', isClosed: true, isWon: true },
  { id: 6, name: 'Closed Lost', isClosed: true, isWon: false }
]

describe('DealSignatureCard', () => {
  it('collects a won-stage close review before converting signed evidence', () => {
    const onConvert = vi.fn()
    render(<DealSignatureCard canWrite deal={{ id: 12, status: 'open' }} dealID={12} isSnapshotPending={false} onConvert={onConvert} onVoid={vi.fn()} requests={[signedRequest]} stages={stages} />)

    fireEvent.click(screen.getByText('Convert signed quote to won'))
    fireEvent.change(screen.getByLabelText('Won stage for Q-12-V1'), { target: { value: '5' } })
    fireEvent.change(screen.getByLabelText('Won reason'), { target: { value: 'solution_fit' } })
    fireEvent.change(screen.getByLabelText('Close notes'), { target: { value: 'Signed scope accepted.' } })
    fireEvent.click(screen.getByRole('button', { name: 'Convert signed quote and hand off client' }))

    expect(onConvert).toHaveBeenCalledWith(41, {
      stageId: '5',
      closeReasonCode: 'solution_fit',
      closeNotes: 'Signed scope accepted.'
    })
    expect(screen.queryByRole('option', { name: 'Closed Lost' })).not.toBeInTheDocument()
  })

  it('shows retained conversion evidence without offering a repeated action', () => {
    render(<DealSignatureCard canWrite deal={{ id: 12, status: 'open' }} dealID={12} isSnapshotPending={false} onConvert={vi.fn()} onVoid={vi.fn()} requests={[{
      ...signedRequest,
      conversionStageId: 5,
      conversionStageName: 'Closed Won',
      conversionCloseReasonLabel: 'Best solution fit',
      conversionCloseNotes: 'Signed scope accepted.',
      convertedByUserName: 'Demo Owner',
      convertedAt: '2026-07-21T08:30:00Z'
    }]} stages={stages} />)

    expect(screen.getByLabelText('Signed quote conversion for Q-12-V1')).toHaveTextContent('Converted to Closed Won')
    expect(screen.getByText(/later stage changes do not erase/i)).toBeInTheDocument()
    expect(screen.queryByText('Convert signed quote to won')).not.toBeInTheDocument()
  })
})

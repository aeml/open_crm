import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ContactLeadScoreEvidenceCard } from './contact_insights'

describe('ContactLeadScoreEvidenceCard', () => {
  it('shows retained score evidence without exposing scoring execution', () => {
    const { rerender } = render(
      <ContactLeadScoreEvidenceCard
        contact={{
          leadScore: 72,
          leadGrade: 'B',
          leadScoredAt: '2026-07-22T14:30:00Z',
          ownerUserName: 'Alex Owner',
        }}
      />,
    )

    expect(screen.getByRole('heading', { name: 'Lead score evidence' })).toBeInTheDocument()
    expect(screen.getByText('Score 72 (B)')).toBeInTheDocument()
    expect(screen.getByText('Owner: Alex Owner')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /evaluate score/i })).not.toBeInTheDocument()

    rerender(<ContactLeadScoreEvidenceCard contact={{}} />)
    expect(screen.queryByRole('heading', { name: 'Lead score evidence' })).not.toBeInTheDocument()
  })
})

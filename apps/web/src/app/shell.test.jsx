import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { AppShell } from './shell'

describe('AppShell', () => {
  it('renders a polished CRM shell with primary navigation', () => {
    render(
      <MemoryRouter>
        <AppShell />
      </MemoryRouter>
    )

    expect(screen.getByRole('heading', { name: /open crm/i })).toBeInTheDocument()
    expect(screen.getByText(/pipeline at a glance/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /clients/i })).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: /contacts/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /log out/i })).toBeInTheDocument()
  })
})

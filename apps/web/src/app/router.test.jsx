import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AppRouter } from './router'

describe('AppRouter', () => {
  it('renders dashboard content at the default route shell', () => {
    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    expect(screen.getByText(/keep the next best action obvious/i)).toBeInTheDocument()
  })

  it('renders the contacts route without blanking the app shell', () => {
    window.history.pushState({}, '', '/contacts')

    render(<AppRouter />)

    expect(screen.getByRole('heading', { name: /contacts/i })).toBeInTheDocument()
    expect(screen.getByText(/keep the right people moving/i)).toBeInTheDocument()
    expect(screen.getByRole('list', { name: /contacts list/i })).toBeInTheDocument()
  })
})

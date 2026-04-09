import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import { AppRouter } from './router'

describe('AppRouter', () => {
  it('renders dashboard content at the default route shell', () => {
    window.history.pushState({}, '', '/dashboard')

    render(<AppRouter />)

    expect(screen.getByText(/keep the next best action obvious/i)).toBeInTheDocument()
  })
})

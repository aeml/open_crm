import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { PagedSearchControls } from './paged_search_controls'

describe('PagedSearchControls', () => {
  it('binds one explicit accessible label to the search input', () => {
    const lookup = {
      appliedQuery: '',
      error: '',
      isLoading: false,
      query: '',
      reset: vi.fn(),
      search: vi.fn(),
      setQuery: vi.fn()
    }
    render(
      <PagedSearchControls
        hint="Find an existing contact without loading the entire workspace."
        id="company-contact-link-search"
        label="Search workspace contacts"
        lookup={lookup}
        placeholder="Search contacts to link"
      />
    )

    const input = screen.getByRole('textbox', { name: 'Search workspace contacts' })
    expect(input).toHaveAccessibleDescription('Find an existing contact without loading the entire workspace.')
    fireEvent.change(input, { target: { value: 'buyer@example.test' } })
    fireEvent.click(screen.getByRole('button', { name: 'Search' }))
    expect(lookup.setQuery).toHaveBeenCalledWith('buyer@example.test')
    expect(lookup.search).toHaveBeenCalledTimes(1)
  })
})

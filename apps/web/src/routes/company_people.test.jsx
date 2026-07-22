import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { CompanyPeople } from './company_people'
import { emptyLinkedPersonForm } from './company_view'

function lookup(contacts = []) {
  return {
    appliedQuery: '', contacts, error: '', isLoading: false,
    loadMore: vi.fn(), meta: { page: 1, pageSize: 20, total: contacts.length },
    query: '', reset: vi.fn(), search: vi.fn(), setQuery: vi.fn()
  }
}

describe('CompanyPeople', () => {
  it('offers an explicit replacement without allowing an individual client to reach zero links', () => {
    const existing = { id: 7, firstName: 'Morgan', lastName: 'Lee', email: 'morgan@example.test', isPrimary: true }
    const replacement = { id: 8, firstName: 'Ava', lastName: 'Stone', email: 'ava@example.test' }
    const onToggleLinkForm = vi.fn()
    const props = {
      canWrite: true,
      company: { id: 6, clientType: 'individual' },
      contacts: [existing],
      contactLookup: lookup([replacement]),
      customDefinitions: [],
      directory: lookup([existing]),
      form: emptyLinkedPersonForm,
      isLinking: false,
      isSaving: false,
      linkForm: { contactId: '8', relationshipTitle: '', isPrimary: false },
      onLinkSubmit: vi.fn(),
      onMakePrimary: vi.fn(),
      onOpenContact: vi.fn(),
      onSetForm: vi.fn(),
      onSetLinkForm: vi.fn(),
      onSubmit: vi.fn(),
      onToggleForm: vi.fn(),
      onToggleLinkForm,
      onUnlink: vi.fn(),
      showForm: false,
      showLinkForm: false
    }
    const { rerender } = render(<CompanyPeople {...props} />)

    fireEvent.click(screen.getByRole('button', { name: 'Replace linked person' }))
    expect(onToggleLinkForm).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('button', { name: 'Add person' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Unlink' })).not.toBeInTheDocument()

    rerender(<CompanyPeople {...props} showLinkForm />)
    expect(screen.getByRole('combobox', { name: 'Existing contact' })).toHaveValue('8')
    expect(screen.queryByRole('checkbox', { name: 'Make this the primary contact' })).not.toBeInTheDocument()
    fireEvent.submit(screen.getByRole('button', { name: 'Replace linked person' }).closest('form'))
    expect(props.onLinkSubmit).toHaveBeenCalledTimes(1)
  })
})

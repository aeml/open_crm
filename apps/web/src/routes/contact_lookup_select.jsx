import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { applyLinkedContactSelection, isIndividualClient, linkedContactFieldHint, linkedContactFieldLabel, parseLinkedContactIDs } from './company_view'
import { PagedSearchControls } from './paged_search_controls'

export function ContactLookupSelect({ form, lookup, onSetForm }) {
  const selectedID = String(parseLinkedContactIDs(form.linkedContactIDs)[0] || '')
  const selectedIsVisible = lookup.contacts.some((contact) => String(contact.id) === selectedID)
  const hasMore = lookup.contacts.length < lookup.meta.total
  return (
    <div className="card-stack">
      <PagedSearchControls
        hint="Search by name or contact details. Results are limited to this workspace."
        id="company-contact-lookup-search"
        label="Search contacts"
        lookup={lookup}
        placeholder="Search contacts"
      />
      <Field label={linkedContactFieldLabel(form.clientType)} hint={linkedContactFieldHint(form.clientType)}>
        <select className="text-input" value={selectedID} onChange={(event) => onSetForm((current) => applyLinkedContactSelection(current, lookup.contacts, event.target.value))}>
          <option value="">{isIndividualClient(form.clientType) ? 'Select person record' : 'No linked contact'}</option>
          {selectedID && !selectedIsVisible ? <option value={selectedID}>Previously selected contact</option> : null}
          {lookup.contacts.map((contact) => <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>)}
        </select>
      </Field>
      <p className="field-hint" role="status">Showing {lookup.contacts.length} of {lookup.meta.total} matching contacts.</p>
      {hasMore ? <Button className="button-secondary" type="button" onClick={lookup.loadMore} disabled={lookup.isLoading}>{lookup.isLoading ? 'Loading…' : 'Load more contacts'}</Button> : null}
    </div>
  )
}

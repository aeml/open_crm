import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { EmptyState } from '../components/ui/empty_state'
import { Field } from '../components/ui/field'
import { SavedViews } from '../components/ui/saved_views'
import { BulkActions } from '../components/ui/bulk_actions'
import { CustomFieldFilter } from '../components/ui/custom_field_filter'
import { CustomFieldValue } from '../components/ui/custom_fields_form'
import { attributionSummary, hasAttribution, hasLeadScore, leadScoreLabel } from './contact_insights'
import { formatContactAddress, fullContactName } from './contact_view'

export function ContactListCard({
  canWrite,
  bulkActions,
  contacts,
  currentUserId,
  customDefinitions,
  customFilter,
  duplicateCandidate,
  duplicateSearch,
  error,
  exportURL,
  hasFilter,
  isLoading,
  meta,
  onApplyOwnerFilter,
  onApplyCustomFilter,
  onApplySavedView,
  onClearFilters,
  onClearCustomFilter,
  onCreate,
  onDuplicateSearch,
  onOpenContact,
  onOpenDuplicate,
  onOwnerFilterChange,
  onReload,
  onSearchChange,
  ownerFilter,
  search,
  userOptions
}) {
  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h2>Contacts</h2>
            <p>Keep the right people moving without a bloated CRM mess.</p>
          </div>
          <div className="button-row">
            <a className="button button-secondary" href={exportURL}>Export CSV</a>
            {canWrite ? <Button onClick={onCreate}>Add contact</Button> : null}
          </div>
        </div>
        <p className="field-hint">CSV exports include up to 10,000 matching contacts. Apply filters first for larger sets.</p>
        <Field label="Search contacts">
          <input className="text-input" type="search" value={search} onChange={onSearchChange} />
        </Field>
        <SavedViews entityType="contacts" canManage={canWrite} currentFilters={{ q: search, owner: ownerFilter, customField: customFilter.fieldKey, customOperator: customFilter.operator, customValue: customFilter.value }} onApply={onApplySavedView} defaultName="Contact view" />
        <Field label="Owner filter">
          <div className="button-row">
            <select className="text-input" value={ownerFilter} onChange={onOwnerFilterChange}>
              <option value="all">All owners</option>
              <option value="unassigned">Unassigned</option>
              {userOptions.map((user) => (
                <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
              ))}
            </select>
            {currentUserId ? (
              <Button className={ownerFilter === currentUserId ? '' : 'button-secondary'} type="button" onClick={() => onApplyOwnerFilter(currentUserId)}>
                Mine
              </Button>
            ) : null}
            <Button className={ownerFilter === 'unassigned' ? '' : 'button-secondary'} type="button" onClick={() => onApplyOwnerFilter('unassigned')}>
              Unassigned
            </Button>
          </div>
        </Field>
        <CustomFieldFilter definitions={customDefinitions} value={customFilter} onApply={onApplyCustomFilter} onClear={onClearCustomFilter} />
        {isLoading ? <p className="field-hint">Loading contacts...</p> : null}
        {error ? (
          <div className="card-stack">
            <p className="form-error">{error}</p>
            <div><Button className="button-secondary" type="button" onClick={onReload}>Retry contacts</Button></div>
            {duplicateCandidate ? <div><Button className="button-secondary" onClick={onOpenDuplicate}>Open matching contact</Button></div> : null}
            {duplicateSearch ? (
              <div><Button className="button-secondary" onClick={onDuplicateSearch}>Search existing contacts for {duplicateSearch}</Button></div>
            ) : null}
          </div>
        ) : null}
        {canWrite ? <BulkActions {...bulkActions} /> : null}
        <div className="record-list" role="list" aria-label="Contacts list">
          {!isLoading && contacts.length === 0 ? (
            <EmptyState
              title={hasFilter ? 'No contacts match the current filters.' : 'No contacts yet.'}
              description={hasFilter ? 'Try a different name, email, phone number, or change the owner filter.' : 'Add the first person you need to follow up with. You can link contacts to clients, deals, notes, and tasks later.'}
              actionLabel={hasFilter ? 'Clear filters' : (canWrite ? 'Create first contact' : '')}
              onAction={hasFilter ? onClearFilters : onCreate}
            />
          ) : contacts.map((contact) => (
            <article className="record-row" key={contact.id} role="listitem">
              <div>
                {canWrite ? <input type="checkbox" aria-label={`Select ${fullContactName(contact)}`} checked={bulkActions.selectedIds.includes(contact.id)} onChange={() => bulkActions.onSelectionChange((current) => current.includes(contact.id) ? current.filter((id) => id !== contact.id) : [...current, contact.id])} /> : null}
                <button className="button button-ghost contact-link" type="button" onClick={() => onOpenContact(contact)}>{fullContactName(contact)}</button>
                <p>{contact.jobTitle || 'No title'}</p>
              </div>
              <div>
                <p>{contact.email || formatContactAddress(contact) || 'No contact details'}</p>
                <p>{contact.status}</p>
                <p className="field-hint">{contact.ownerUserName || 'Unassigned'}</p>
                {hasLeadScore(contact) ? <p className="field-hint">Lead score {leadScoreLabel(contact)}</p> : null}
                {hasAttribution(contact) ? <p className="field-hint">{attributionSummary(contact)}</p> : null}
                {customDefinitions.filter((definition) => definition.showInList).map((definition) => (
                  <p className="field-hint" key={definition.id}><CustomFieldValue definition={definition} value={contact.customFields?.[definition.fieldKey]} /></p>
                ))}
              </div>
            </article>
          ))}
        </div>
        <p className="field-hint">Showing {contacts.length} of {meta.total} contacts.</p>
      </div>
    </Card>
  )
}

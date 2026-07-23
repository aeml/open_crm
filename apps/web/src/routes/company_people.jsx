import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { CustomFieldsForm } from '../components/ui/custom_fields_form'
import { ControlledTextField, Field } from '../components/ui/field'
import { isIndividualClient } from './company_view'
import { PagedSearchControls } from './paged_search_controls'

export function CompanyPeople({
  canWrite,
  company,
  contacts,
  contactLookup,
  customDefinitions,
  directory,
  form,
  isLinking,
  isSaving,
  linkForm,
  onLinkSubmit,
  onMakePrimary,
  onOpenContact,
  onSetForm,
  onSetLinkForm,
  onSubmit,
  onToggleLinkForm,
  onToggleForm,
  onUnlink,
  showForm,
  showLinkForm
}) {
  const isIndividual = isIndividualClient(company.clientType)

  return (
    <Card className="company-people-card">
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>People</h3>
            <p>{isIndividual ? 'Manage the linked person for this client.' : 'Add and manage the people tied to this client.'}</p>
          </div>
          {canWrite ? (
            <div className="button-row">
              <Button className="button-secondary" type="button" onClick={onToggleLinkForm}>
                {showLinkForm ? 'Cancel linking' : (isIndividual ? 'Replace linked person' : 'Link existing contact')}
              </Button>
              {!isIndividual ? <Button className="button-secondary" type="button" onClick={onToggleForm}>
                {showForm ? 'Cancel new person' : 'Add person'}
              </Button> : null}
            </div>
          ) : null}
        </div>
        <div className="card-stack" role="search" aria-label="Search linked people">
          <PagedSearchControls
            hint="Search by name, email, or relationship title."
            id="company-linked-people-search"
            label="Search linked people"
            lookup={directory}
            placeholder="Search linked people"
          />
          <p className="field-hint" role="status">Showing {contacts.length} of {directory.meta.total} linked people{directory.appliedQuery ? ` matching “${directory.appliedQuery}”` : ''}.</p>
        </div>
        {showLinkForm && canWrite ? (
          <form className="auth-form" onSubmit={onLinkSubmit}>
            <PagedSearchControls
              hint={isIndividual ? 'Choose the one person record that should represent this client.' : 'Find an existing contact without loading the entire workspace.'}
              id="company-contact-link-search"
              label="Search workspace contacts"
              lookup={contactLookup}
              placeholder="Search contacts to link"
            />
            <Field label="Existing contact">
              <select className="text-input" value={linkForm.contactId} onChange={(event) => onSetLinkForm((current) => ({ ...current, contactId: event.target.value }))} required>
                <option value="">Select contact</option>
                {linkForm.contactId && !contactLookup.contacts.some((contact) => String(contact.id) === String(linkForm.contactId)) ? <option value={linkForm.contactId}>Previously selected contact</option> : null}
                {contactLookup.contacts.map((contact) => <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>)}
              </select>
            </Field>
            <p className="field-hint" role="status">Showing {contactLookup.contacts.length} of {contactLookup.meta.total} matching contacts.</p>
            {contactLookup.contacts.length < contactLookup.meta.total ? <Button className="button-secondary" type="button" onClick={contactLookup.loadMore} disabled={contactLookup.isLoading}>Load more contacts</Button> : null}
            <ControlledTextField form={linkForm} label="Relationship title" maxLength={200} name="relationshipTitle" setForm={onSetLinkForm} />
            {!isIndividual ? <label className="field-hint">
              <input type="checkbox" checked={linkForm.isPrimary} onChange={(event) => onSetLinkForm((current) => ({ ...current, isPrimary: event.target.checked }))} /> Make this the primary contact
            </label> : null}
            <Button type="submit" disabled={isLinking || !linkForm.contactId}>{isLinking ? 'Saving…' : (isIndividual ? 'Replace linked person' : 'Link contact')}</Button>
          </form>
        ) : null}
        {showForm && !isIndividual && canWrite ? (
          <form className="auth-form" onSubmit={onSubmit}>
            <ControlledTextField form={form} label="First name" name="firstName" required setForm={onSetForm} />
            <ControlledTextField form={form} label="Last name" name="lastName" required setForm={onSetForm} />
            <ControlledTextField form={form} label="Email" name="email" setForm={onSetForm} type="email" />
            <ControlledTextField form={form} label="Phone" name="phone" setForm={onSetForm} />
            <ControlledTextField form={form} label="Job title" name="jobTitle" setForm={onSetForm} />
            <CustomFieldsForm definitions={customDefinitions} values={form.customFields} onChange={(customFields) => onSetForm((current) => ({ ...current, customFields }))} />
            <Button type="submit" disabled={isSaving}>{isSaving ? 'Saving…' : 'Save person'}</Button>
          </form>
        ) : null}
        <div className="record-list" role="list" aria-label="Linked contacts list">
          {contacts.length === 0 ? (
            <article className="record-row" role="listitem">
              <div>
                <p>{isIndividual ? 'No linked person yet.' : 'No linked people yet.'}</p>
              </div>
            </article>
          ) : contacts.map((contact) => (
            <article className="record-row" key={contact.id} role="listitem">
              <div>
                <button className="button button-ghost contact-link" type="button" onClick={() => onOpenContact(contact.id)}>
                  {contact.firstName} {contact.lastName}
                </button>
                <p>{contact.relationshipTitle || (isIndividual ? 'Client record' : 'Linked contact')}</p>
              </div>
              <div>
                <p>{contact.email}</p>
                <p>{contact.isPrimary ? 'Primary' : 'Linked'}</p>
                {canWrite && !isIndividual ? (
                  <div className="button-row">
                    {!contact.isPrimary ? <Button className="button-ghost" type="button" onClick={() => onMakePrimary(contact)} disabled={isLinking}>Make primary</Button> : null}
                    <Button className="button-ghost" type="button" onClick={() => onUnlink(contact)} disabled={isLinking}>Unlink</Button>
                  </div>
                ) : null}
              </div>
            </article>
          ))}
        </div>
        {contacts.length < directory.meta.total ? (
          <Button className="button-secondary" type="button" onClick={directory.loadMore} disabled={directory.isLoading}>
            {directory.isLoading ? 'Loading…' : 'Load more linked people'}
          </Button>
        ) : null}
      </div>
    </Card>
  )
}

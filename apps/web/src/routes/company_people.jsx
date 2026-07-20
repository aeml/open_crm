import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { CustomFieldsForm } from '../components/ui/custom_fields_form'
import { Field } from '../components/ui/field'
import { isIndividualClient, normalizeClientType } from './company_view'

export function CompanyPeople({
  canWrite,
  company,
  contacts,
  customDefinitions,
  form,
  isSaving,
  onOpenContact,
  onSetForm,
  onSubmit,
  onToggleForm,
  showForm
}) {
  const isIndividual = isIndividualClient(company.clientType)

  return (
    <Card>
      <div className="card-stack">
        <div className="section-header">
          <div>
            <h3>People</h3>
            <p>{isIndividual ? 'Manage the linked person for this client.' : 'Add and manage the people tied to this client.'}</p>
          </div>
          {!isIndividual && canWrite ? (
            <Button className="button-secondary" onClick={onToggleForm}>
              {showForm ? 'Cancel' : 'Add person'}
            </Button>
          ) : null}
        </div>
        {showForm && !isIndividual && canWrite ? (
          <form className="auth-form" onSubmit={onSubmit}>
            <Field label="First name">
              <input className="text-input" value={form.firstName} onChange={(event) => onSetForm((current) => ({ ...current, firstName: event.target.value }))} required />
            </Field>
            <Field label="Last name">
              <input className="text-input" value={form.lastName} onChange={(event) => onSetForm((current) => ({ ...current, lastName: event.target.value }))} required />
            </Field>
            <Field label="Email">
              <input className="text-input" type="email" value={form.email} onChange={(event) => onSetForm((current) => ({ ...current, email: event.target.value }))} />
            </Field>
            <Field label="Phone">
              <input className="text-input" value={form.phone} onChange={(event) => onSetForm((current) => ({ ...current, phone: event.target.value }))} />
            </Field>
            <Field label="Job title">
              <input className="text-input" value={form.jobTitle} onChange={(event) => onSetForm((current) => ({ ...current, jobTitle: event.target.value }))} />
            </Field>
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
              </div>
            </article>
          ))}
        </div>
      </div>
    </Card>
  )
}

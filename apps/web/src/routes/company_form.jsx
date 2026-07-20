import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { CustomFieldsForm } from '../components/ui/custom_fields_form'
import {
  applyLinkedContactSelection,
  isIndividualClient,
  limitLinkedContacts,
  linkedContactFieldHint,
  linkedContactFieldLabel,
  nameFieldLabel,
  normalizeClientType,
  phoneFieldLabel
} from './company_view'

export function CompanyForm({ canSubmit = true, contacts, customDefinitions = [], form, includeStatus = false, isSubmitting = false, onSetForm, onSubmit, submitLabel }) {
  const setField = (name) => (event) => onSetForm((current) => ({ ...current, [name]: event.target.value }))
  return (
    <form className="auth-form" onSubmit={onSubmit}>
      <Field label="Client type">
        <select
          className="text-input"
          value={form.clientType}
          onChange={(event) => onSetForm((current) => ({
            ...current,
            clientType: event.target.value,
            linkedContactIDs: limitLinkedContacts(event.target.value, current.linkedContactIDs),
            customFields: {}
          }))}
        >
          <option value="organization">Organization</option>
          <option value="individual">Individual</option>
        </select>
      </Field>
      <Field label={nameFieldLabel(form.clientType)}>
        <input className="text-input" value={form.name} onChange={setField('name')} required />
      </Field>
      <Field label={phoneFieldLabel(form.clientType)}>
        <input className="text-input" value={form.phone} onChange={setField('phone')} />
      </Field>
      {isIndividualClient(form.clientType) ? (
        <Field label="Email">
          <input className="text-input" type="email" value={form.email} onChange={setField('email')} />
        </Field>
      ) : null}
      <Field label="Address line 1">
        <input className="text-input" value={form.addressLine1} onChange={setField('addressLine1')} />
      </Field>
      <Field label="Address line 2">
        <input className="text-input" value={form.addressLine2} onChange={setField('addressLine2')} />
      </Field>
      <Field label="City">
        <input className="text-input" value={form.city} onChange={setField('city')} />
      </Field>
      <Field label="State">
        <input className="text-input" value={form.state} onChange={setField('state')} />
      </Field>
      <Field label="Postal code">
        <input className="text-input" value={form.postalCode} onChange={setField('postalCode')} />
      </Field>
      <Field label="Country">
        <input className="text-input" value={form.country} onChange={setField('country')} />
      </Field>
      {includeStatus ? (
        <Field label="Status">
          <select className="text-input" value={form.status} onChange={setField('status')}>
            <option value="prospect">Prospect</option>
            <option value="customer">Customer</option>
            <option value="lead">Lead</option>
          </select>
        </Field>
      ) : null}
      <Field label={linkedContactFieldLabel(form.clientType)} hint={linkedContactFieldHint(form.clientType)}>
        <select className="text-input" value={form.linkedContactIDs} onChange={(event) => onSetForm((current) => applyLinkedContactSelection(current, contacts, event.target.value))}>
          <option value="">{normalizeClientType(form.clientType) === 'individual' ? 'Select person record' : 'No linked contact'}</option>
          {contacts.map((contact) => <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>)}
        </select>
      </Field>
      {!isIndividualClient(form.clientType) ? (
        <>
          <Field label="Industry">
            <input className="text-input" value={form.industry} onChange={setField('industry')} />
          </Field>
          <Field label="Website" hint="Company site, like https://acme.com.">
            <input className="text-input" value={form.website} onChange={setField('website')} />
          </Field>
        </>
      ) : null}
      {!isIndividualClient(form.clientType) ? <CustomFieldsForm definitions={customDefinitions} values={form.customFields || {}} onChange={(customFields) => onSetForm((current) => ({ ...current, customFields }))} /> : null}
      {canSubmit ? <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Saving…' : submitLabel}</Button> : null}
    </form>
  )
}

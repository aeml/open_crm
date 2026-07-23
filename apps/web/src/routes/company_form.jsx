import { Button } from '../components/ui/button'
import { ControlledTextField, Field } from '../components/ui/field'
import { CustomFieldsForm } from '../components/ui/custom_fields_form'
import { ContactLookupSelect } from './contact_lookup_select'
import {
  isIndividualClient,
  limitLinkedContacts,
  nameFieldLabel,
  phoneFieldLabel
} from './company_view'

export function CompanyForm({ canSubmit = true, contactLookup, customDefinitions = [], form, includeLinkedContact = true, includeStatus = false, isSubmitting = false, onSetForm, onSubmit, submitLabel }) {
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
      <ControlledTextField form={form} label={nameFieldLabel(form.clientType)} name="name" required setForm={onSetForm} />
      <ControlledTextField form={form} label={phoneFieldLabel(form.clientType)} name="phone" setForm={onSetForm} />
      {isIndividualClient(form.clientType) ? (
        <ControlledTextField form={form} label="Email" name="email" setForm={onSetForm} type="email" />
      ) : null}
      <ControlledTextField form={form} label="Address line 1" name="addressLine1" setForm={onSetForm} />
      <ControlledTextField form={form} label="Address line 2" name="addressLine2" setForm={onSetForm} />
      <ControlledTextField form={form} label="City" name="city" setForm={onSetForm} />
      <ControlledTextField form={form} label="State" name="state" setForm={onSetForm} />
      <ControlledTextField form={form} label="Postal code" name="postalCode" setForm={onSetForm} />
      <ControlledTextField form={form} label="Country" name="country" setForm={onSetForm} />
      {includeStatus ? (
        <Field label="Status">
          <select className="text-input" value={form.status} onChange={(event) => onSetForm((current) => ({ ...current, status: event.target.value }))}>
            <option value="prospect">Prospect</option>
            <option value="customer">Customer</option>
            <option value="lead">Lead</option>
          </select>
        </Field>
      ) : null}
      {includeLinkedContact && contactLookup ? <ContactLookupSelect form={form} lookup={contactLookup} onSetForm={onSetForm} /> : null}
      {!isIndividualClient(form.clientType) ? (
        <>
          <ControlledTextField form={form} label="Industry" name="industry" setForm={onSetForm} />
          <ControlledTextField form={form} hint="Company site, like https://acme.com." label="Website" name="website" setForm={onSetForm} />
        </>
      ) : null}
      {!isIndividualClient(form.clientType) ? <CustomFieldsForm definitions={customDefinitions} values={form.customFields || {}} onChange={(customFields) => onSetForm((current) => ({ ...current, customFields }))} /> : null}
      {canSubmit ? <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Saving…' : submitLabel}</Button> : null}
    </form>
  )
}

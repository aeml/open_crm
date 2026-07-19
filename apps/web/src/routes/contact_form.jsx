import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { CustomFieldsForm } from '../components/ui/custom_fields_form'

export function ContactForm({ canSubmit = true, customDefinitions = [], form, includeStatus = false, onSetForm, onSubmit, submitLabel }) {
  const setField = (name) => (event) => onSetForm((current) => ({ ...current, [name]: event.target.value }))
  return (
    <form className="auth-form" onSubmit={onSubmit}>
      <Field label="First name">
        <input className="text-input" value={form.firstName} onChange={setField('firstName')} required />
      </Field>
      <Field label="Last name">
        <input className="text-input" value={form.lastName} onChange={setField('lastName')} required />
      </Field>
      <Field label="Email">
        <input className="text-input" type="email" value={form.email} onChange={setField('email')} />
      </Field>
      <Field label="Phone">
        <input className="text-input" value={form.phone} onChange={setField('phone')} />
      </Field>
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
      <Field label="Job title">
        <input className="text-input" value={form.jobTitle} onChange={setField('jobTitle')} />
      </Field>
      {includeStatus ? (
        <Field label="Status">
          <select className="text-input" value={form.status} onChange={setField('status')}>
            <option value="lead">Lead</option>
            <option value="customer">Customer</option>
            <option value="prospect">Prospect</option>
          </select>
        </Field>
      ) : null}
      <CustomFieldsForm definitions={customDefinitions} values={form.customFields || {}} onChange={(customFields) => onSetForm((current) => ({ ...current, customFields }))} />
      {canSubmit ? <Button type="submit">{submitLabel}</Button> : null}
    </form>
  )
}

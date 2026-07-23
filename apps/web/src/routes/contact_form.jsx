import { Button } from '../components/ui/button'
import { ControlledTextField, Field } from '../components/ui/field'
import { CustomFieldsForm } from '../components/ui/custom_fields_form'

export function ContactForm({ canSubmit = true, customDefinitions = [], form, includeStatus = false, isSubmitting = false, onSetForm, onSubmit, submitLabel }) {
  return (
    <form className="auth-form" onSubmit={onSubmit}>
      <ControlledTextField form={form} label="First name" name="firstName" required setForm={onSetForm} />
      <ControlledTextField form={form} label="Last name" name="lastName" required setForm={onSetForm} />
      <ControlledTextField form={form} label="Email" name="email" setForm={onSetForm} type="email" />
      <ControlledTextField form={form} label="Phone" name="phone" setForm={onSetForm} />
      <ControlledTextField form={form} label="Address line 1" name="addressLine1" setForm={onSetForm} />
      <ControlledTextField form={form} label="Address line 2" name="addressLine2" setForm={onSetForm} />
      <ControlledTextField form={form} label="City" name="city" setForm={onSetForm} />
      <ControlledTextField form={form} label="State" name="state" setForm={onSetForm} />
      <ControlledTextField form={form} label="Postal code" name="postalCode" setForm={onSetForm} />
      <ControlledTextField form={form} label="Country" name="country" setForm={onSetForm} />
      <ControlledTextField form={form} label="Job title" name="jobTitle" setForm={onSetForm} />
      {includeStatus ? (
        <Field label="Status">
          <select className="text-input" value={form.status} onChange={(event) => onSetForm((current) => ({ ...current, status: event.target.value }))}>
            <option value="lead">Lead</option>
            <option value="customer">Customer</option>
            <option value="prospect">Prospect</option>
          </select>
        </Field>
      ) : null}
      <CustomFieldsForm definitions={customDefinitions} values={form.customFields || {}} onChange={(customFields) => onSetForm((current) => ({ ...current, customFields }))} />
      {canSubmit ? <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Saving…' : submitLabel}</Button> : null}
    </form>
  )
}

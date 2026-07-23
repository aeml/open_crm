import { Button } from '../components/ui/button'
import { ControlledTextField, Field } from '../components/ui/field'

function defaultEntityId(entityType, dealOptions, companyOptions, contactOptions) {
  if (entityType === 'deal') return dealOptions[0] ? String(dealOptions[0].id) : ''
  if (entityType === 'company') return companyOptions[0] ? String(companyOptions[0].id) : ''
  if (entityType === 'contact') return contactOptions[0] ? String(contactOptions[0].id) : ''
  return ''
}

export function TaskForm({
  canArchive = false,
  canSubmit = true,
  companyOptions = [],
  contactOptions = [],
  dealOptions = [],
  form,
  isSubmitting = false,
  labels,
  onArchive,
  onSetForm,
  onSubmit,
  showEntityFields = false,
  showStatusFields = false,
  submitLabel,
  userOptions = []
}) {
  return (
    <form className="auth-form" onSubmit={onSubmit}>
      <ControlledTextField form={form} label="Task title" name="title" required setForm={onSetForm} />
      {showEntityFields ? (
        <Field label={labels.entityTypeLabel}>
          <select className="text-input" value={form.entityType} onChange={(event) => onSetForm((current) => ({ ...current, entityType: event.target.value, entityId: defaultEntityId(event.target.value, dealOptions, companyOptions, contactOptions) }))}>
            <option value="deal">{labels.dealOption}</option>
            <option value="company">{labels.companyLabel}</option>
            <option value="contact">Contact</option>
          </select>
        </Field>
      ) : null}
      {showEntityFields && form.entityType === 'deal' ? (
        <Field label={labels.dealOption}>
          <select className="text-input" value={form.entityId} onChange={(event) => onSetForm((current) => ({ ...current, entityId: event.target.value }))} required>
            {dealOptions.map((deal) => (
              <option key={deal.id} value={deal.id}>{deal.name}</option>
            ))}
          </select>
        </Field>
      ) : null}
      {showEntityFields && form.entityType === 'company' ? (
        <Field label={labels.companyLabel}>
          <select className="text-input" value={form.entityId} onChange={(event) => onSetForm((current) => ({ ...current, entityId: event.target.value }))} required>
            {companyOptions.map((company) => (
              <option key={company.id} value={company.id}>{company.name}</option>
            ))}
          </select>
        </Field>
      ) : null}
      {showEntityFields && form.entityType === 'contact' ? (
        <Field label="Contact">
          <select className="text-input" value={form.entityId} onChange={(event) => onSetForm((current) => ({ ...current, entityId: event.target.value }))} required>
            {contactOptions.map((contact) => (
              <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>
            ))}
          </select>
        </Field>
      ) : null}
      <ControlledTextField form={form} label="Description" multiline name="description" setForm={onSetForm} />
      <Field label="Assigned to">
        <select className="text-input" value={form.assignedToUserId} onChange={(event) => onSetForm((current) => ({ ...current, assignedToUserId: event.target.value }))}>
          {userOptions.map((user) => (
            <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
          ))}
        </select>
      </Field>
      {showStatusFields ? (
        <Field label="Status">
          <select className="text-input" value={form.status} onChange={(event) => onSetForm((current) => ({ ...current, status: event.target.value }))}>
            <option value="open">Open</option>
            <option value="completed">Completed</option>
          </select>
        </Field>
      ) : null}
      <ControlledTextField form={form} label="Due at" name="dueAt" setForm={onSetForm} type="datetime-local" />
      {showStatusFields ? (
        <ControlledTextField form={form} label="Completed at" name="completedAt" setForm={onSetForm} type="datetime-local" />
      ) : null}
      {canSubmit ? <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Saving…' : submitLabel}</Button> : null}
      {canArchive ? <Button className="button-danger" type="button" disabled={isSubmitting} onClick={onArchive}>Archive task</Button> : null}
    </form>
  )
}

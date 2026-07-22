import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'

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
      <Field label="Task title">
        <input className="text-input" value={form.title} onChange={(event) => onSetForm((current) => ({ ...current, title: event.target.value }))} required />
      </Field>
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
      <Field label="Description">
        <textarea className="text-input" value={form.description} onChange={(event) => onSetForm((current) => ({ ...current, description: event.target.value }))} />
      </Field>
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
      <Field label="Due at">
        <input className="text-input" type="datetime-local" value={form.dueAt} onChange={(event) => onSetForm((current) => ({ ...current, dueAt: event.target.value }))} />
      </Field>
      {showStatusFields ? (
        <Field label="Completed at">
          <input className="text-input" type="datetime-local" value={form.completedAt} onChange={(event) => onSetForm((current) => ({ ...current, completedAt: event.target.value }))} />
        </Field>
      ) : null}
      {canSubmit ? <Button type="submit" disabled={isSubmitting}>{isSubmitting ? 'Saving…' : submitLabel}</Button> : null}
      {canArchive ? <Button className="button-danger" type="button" disabled={isSubmitting} onClick={onArchive}>Archive task</Button> : null}
    </form>
  )
}

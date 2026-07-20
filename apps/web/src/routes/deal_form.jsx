import { Button } from '../components/ui/button'
import { Field } from '../components/ui/field'
import { CloseReviewFields, emptyCloseReview, stageOutcome } from './deal_close_review'
import { stageLabel } from './deal_view'

export function DealForm({
  canSubmit = true,
  companies,
  contacts,
  form,
  labels,
  onSetForm,
  onSubmit,
  pipelineFilter = 'all',
  showStage = false,
  stages = [],
  submitLabel,
  users
}) {
  const selectedStage = stages.find((stage) => String(stage.id) === String(form.stageId))

  return (
    <form className="auth-form" aria-label={showStage ? undefined : 'Deal details form'} onSubmit={onSubmit}>
      <Field label={`${labels.singular} name`}>
        <input className="text-input" value={form.name} onChange={(event) => onSetForm((current) => ({ ...current, name: event.target.value }))} required />
      </Field>
      {showStage ? (
        <>
          <Field label="Stage">
            <select className="text-input" value={form.stageId} onChange={(event) => onSetForm((current) => ({ ...current, stageId: event.target.value, ...emptyCloseReview }))}>
              {stages.map((stage) => (
                <option key={stage.id} value={stage.id}>{stageLabel(stage, pipelineFilter)}</option>
              ))}
            </select>
          </Field>
          <CloseReviewFields outcome={stageOutcome(selectedStage)} value={form} onChange={onSetForm} />
        </>
      ) : null}
      <Field label={labels.companyLabel}>
        <select className="text-input" value={form.companyId} onChange={(event) => onSetForm((current) => ({ ...current, companyId: event.target.value }))}>
          <option value="">{labels.companyEmpty}</option>
          {companies.map((company) => (
            <option key={company.id} value={company.id}>{company.name}</option>
          ))}
        </select>
      </Field>
      <Field label={labels.contactLabel}>
        <select className="text-input" value={form.primaryContactId} onChange={(event) => onSetForm((current) => ({ ...current, primaryContactId: event.target.value }))}>
          <option value="">{labels.contactEmpty}</option>
          {contacts.map((contact) => (
            <option key={contact.id} value={contact.id}>{`${contact.firstName} ${contact.lastName}`.trim()}</option>
          ))}
        </select>
      </Field>
      <Field label={labels.valueLabel}>
        <input className="text-input" value={form.valueAmount} onChange={(event) => onSetForm((current) => ({ ...current, valueAmount: event.target.value }))} />
      </Field>
      <Field label="Value currency">
        <input className="text-input" value={form.valueCurrency} onChange={(event) => onSetForm((current) => ({ ...current, valueCurrency: event.target.value }))} />
      </Field>
      <Field label={labels.dateLabel}>
        <input className="text-input" type="date" value={form.expectedCloseDate} onChange={(event) => onSetForm((current) => ({ ...current, expectedCloseDate: event.target.value }))} />
      </Field>
      <Field label="Owner">
        <select className="text-input" value={form.ownerUserId} onChange={(event) => onSetForm((current) => ({ ...current, ownerUserId: event.target.value }))}>
          {users.map((user) => (
            <option key={user.id} value={user.id}>{`${user.firstName} ${user.lastName}`.trim() || user.email}</option>
          ))}
        </select>
      </Field>
      {canSubmit ? <Button type="submit">{submitLabel}</Button> : null}
    </form>
  )
}

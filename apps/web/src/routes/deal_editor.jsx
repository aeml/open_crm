import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { Field } from '../components/ui/field'
import { quotePDFURL } from '../lib/deals'
import { CloseReviewFields, DealCloseSummary, emptyCloseReview, stageOutcome } from './deal_close_review'
import { DealForm } from './deal_form'
import { stageLabel } from './deal_view'

export function DealCreateCard({ companies, contacts, form, labels, onSetForm, onSubmit, pipelineFilter, stages, users }) {
  return (
    <Card>
      <div className="card-stack">
        <div>
          <h2>{labels.createHeading}</h2>
          <p>{labels.createDescription}</p>
        </div>
        <DealForm
          companies={companies}
          contacts={contacts}
          form={form}
          labels={labels}
          onSetForm={onSetForm}
          onSubmit={onSubmit}
          pipelineFilter={pipelineFilter}
          showStage
          stages={stages}
          submitLabel={`Save ${labels.singular.toLowerCase()}`}
          users={users}
        />
      </div>
    </Card>
  )
}

export function DealDetailsEditor({ canWrite, companies, contacts, deal, form, isLoading, labels, onArchive, onSetForm, onSubmit, users }) {
  return (
    <>
      {isLoading ? <p className="field-hint">Loading {labels.singular.toLowerCase()} detail...</p> : null}
      <div className="section-header">
        <div>
          <h2>{deal.name}</h2>
          <p>{deal.companyName || labels.companyEmpty}</p>
        </div>
        <div className="button-row">
          <a className="button button-secondary" href={quotePDFURL(deal.id)}>Download current-data draft PDF</a>
          {canWrite ? <Button className="button-danger" onClick={onArchive}>{labels.archiveAction}</Button> : null}
        </div>
      </div>
      <DealCloseSummary deal={deal} />
      <DealForm
        canSubmit={canWrite}
        companies={companies}
        contacts={contacts}
        form={form}
        labels={labels}
        onSetForm={onSetForm}
        onSubmit={onSubmit}
        submitLabel={`Update ${labels.singular.toLowerCase()}`}
        users={users}
      />
    </>
  )
}

export function DealStageMover({ canWrite, labels, onMove, onSetReview, onSetStage, review, selectedStageId, stages }) {
  if (!canWrite) return null
  const selectedStage = stages.find((stage) => String(stage.id) === String(selectedStageId))

  function handleStageChange(event) {
    onSetStage(event.target.value)
    onSetReview(emptyCloseReview)
  }

  return (
    <>
      <Field label={labels.moveLabel}>
        <select className="text-input" value={selectedStageId} onChange={handleStageChange}>
          {stages.map((stage) => (
            <option key={stage.id} value={stage.id}>{stageLabel(stage, 'all')}</option>
          ))}
        </select>
      </Field>
      <CloseReviewFields outcome={stageOutcome(selectedStage)} value={review} onChange={onSetReview} />
      <Button type="button" onClick={onMove}>{labels.moveAction}</Button>
    </>
  )
}

import { Card } from '../components/ui/card'
import { RecordEmailComposer } from '../components/record_email_composer'
import { sendDealEmail } from '../lib/deals'
import { DealDetailsEditor, DealStageMover } from './deal_editor'
import { DealLineItemsCard, DealSignatureCard } from './deal_quote'
import { DealQuoteVersionsCard } from './deal_quote_versions'
import { RecordWorkCards } from './record_work'

export function DealWorkspace({
  canWrite,
  commercial,
  companies,
  contacts,
  deal,
  detail,
  emailRecipients,
  labels,
  onOpenTasks,
  stage,
  users,
  work
}) {
  return (
    <Card>
      <div className="card-stack">
        <DealDetailsEditor
          canWrite={canWrite}
          companies={companies}
          contacts={contacts}
          deal={deal}
          form={detail.form}
          isLoading={detail.isLoading}
          labels={labels}
          onArchive={detail.onArchive}
          onSetForm={detail.onSetForm}
          onSubmit={detail.onSubmit}
          users={users}
        />
        <DealLineItemsCard
          canWrite={canWrite}
          deal={deal}
          form={commercial.lineItemForm}
          isSaving={commercial.isSnapshotPending}
          items={commercial.lineItems}
          labels={labels}
          onAdd={commercial.handleAddLineItem}
          onCatalogChange={commercial.handleCatalogLineItemChange}
          onRemove={commercial.handleRemoveLineItem}
          onSave={commercial.handleSaveLineItems}
          onSetForm={commercial.setLineItemForm}
          products={commercial.productCatalogItems}
          totals={commercial.lineTotals}
        />
        <DealQuoteVersionsCard
          areLineItemsDirty={commercial.areLineItemsDirty}
          canWrite={canWrite}
          deal={deal}
          deliveringQuoteId={commercial.deliveringQuoteId}
          form={commercial.quoteForm}
          isFinalizing={commercial.isFinalizingQuote}
          isSnapshotPending={commercial.isSnapshotPending}
          lineItems={commercial.lineItems}
          onDeliver={commercial.handleDeliverQuote}
          onFinalize={commercial.handleFinalizeQuote}
          onReissue={commercial.handleReissueQuote}
          onSetForm={commercial.setQuoteForm}
          onResolveDelivery={commercial.handleResolveQuoteDelivery}
          quotes={commercial.quotes}
          resolvingDeliveryId={commercial.resolvingDeliveryId}
          signatureRequests={commercial.signatureRequests}
        />
        <DealSignatureCard
          canWrite={canWrite}
          convertingID={commercial.convertingSignatureRequestId}
          deal={deal}
          dealID={deal.id}
          isSnapshotPending={commercial.isSnapshotPending}
          onConvert={commercial.handleConvertSignatureRequest}
          onVoid={commercial.handleVoidSignatureRequest}
          requests={commercial.signatureRequests}
          stages={stage.stages}
          voidingID={commercial.voidingSignatureRequestId}
        />
        <DealStageMover
          canWrite={canWrite}
          labels={labels}
          onMove={stage.onMove}
          onSetReview={stage.onSetReview}
          onSetStage={stage.onSetStage}
          review={stage.review}
          selectedStageId={stage.selectedStageId}
          stages={stage.stages}
        />
        <RecordEmailComposer
          entityType="deal"
          entityId={deal.id}
          canWrite={canWrite}
          recipientOptions={emailRecipients}
          sendEmail={sendDealEmail}
          emptyMessage="Set a primary contact with an email address before sending email from this deal."
          mergeFieldHint="Merge fields like {{first_name}}, {{deal_name}}, {{deal_stage}}, and {{company_name}} are filled in when the email is sent."
        />
        <RecordWorkCards
          activities={work.activities}
          activityAria={labels.activityAria}
          canWrite={canWrite}
          entityId={deal.id}
          entityType="deal"
          isCreatingNote={work.isCreatingNote}
          isCreatingTask={work.isCreatingTask}
          isLoading={detail.isLoading}
          noteBody={work.noteBody}
          notes={work.notes}
          notesAria={labels.notesAria}
          onCreateNote={work.handleCreateNote}
          onCreateTask={work.handleCreateTask}
          onOpenTasks={onOpenTasks}
          onSetNoteBody={work.setNoteBody}
          onSetTaskForm={work.setTaskForm}
          taskForm={work.taskForm}
          tasks={work.tasks}
          tasksAria={labels.tasksAria}
          users={users}
        />
      </div>
    </Card>
  )
}

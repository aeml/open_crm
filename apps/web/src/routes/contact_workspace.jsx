import { useLayoutEffect, useState } from 'react'
import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { RecordEmailComposer } from '../components/record_email_composer'
import { ClientAccountContext } from './client_account_context'
import { ClientReviewSchedule } from './client_review_schedule'
import { ContactSequencesCard } from './contact_communications'
import { ContactFoundationCommunications } from './contact_foundation_communications'
import { ContactForm } from './contact_form'
import { ContactAttributionCard, ContactLeadScoreEvidenceCard } from './contact_insights'
import { formatContactAddress, fullContactName } from './contact_view'
import { RecordWorkCards } from './record_work'
import { TouchpointSummary } from './touchpoint_summary'

const showFoundationCommunications = import.meta.env.DEV

export function ContactCreateWorkspace({ customDefinitions, form, isSaving, onSetForm, onSubmit }) {
  return (
    <Card>
      <div className="card-stack">
        <div>
          <h2>New contact</h2>
          <p>Add the next person you need to move through the pipeline.</p>
        </div>
        <ContactForm customDefinitions={customDefinitions} form={form} isSubmitting={isSaving} onSetForm={onSetForm} onSubmit={onSubmit} submitLabel="Save contact" />
      </div>
    </Card>
  )
}

export function ContactWorkspace({
  canWrite,
  contact,
  customDefinitions,
  deals,
  form,
  isArchiving,
  isLoading,
  isSaving,
  onArchive,
  onCreateDeal,
  onError,
  onOpenDeal,
  onOpenTasks,
  onReviewChanged,
  onSetForm,
  onUpdate,
  outreach,
  pipelineLabels,
  users,
  work
}) {
  const [foundationCommunicationsSnapshot, setFoundationCommunicationsSnapshot] = useState('')
  const [emailDeliveryRevision, setEmailDeliveryRevision] = useState(0)

  useLayoutEffect(() => {
    setFoundationCommunicationsSnapshot('')
    setEmailDeliveryRevision(0)
  }, [contact.id])

  return (
    <Card>
      <div className="card-stack">
        {isLoading ? <p className="field-hint">Loading contact detail...</p> : null}
        <div className="section-header">
          <div>
            <h2>{fullContactName(contact)}</h2>
            <p>{contact.email || formatContactAddress(contact) || contact.phone}</p>
          </div>
          {canWrite ? (
            <Button className="button-danger" disabled={isArchiving || isSaving} onClick={onArchive}>
              {isArchiving ? 'Archiving…' : 'Archive contact'}
            </Button>
          ) : null}
        </div>
        <ContactLeadScoreEvidenceCard contact={contact} />
        <ContactForm
          canSubmit={canWrite}
          customDefinitions={customDefinitions}
          form={form}
          includeStatus
          isSubmitting={isSaving}
          onSetForm={onSetForm}
          onSubmit={onUpdate}
          submitLabel="Update contact"
        />
        <ContactAttributionCard contact={contact} />
        <ClientAccountContext
          canWrite={canWrite}
          deals={deals}
          isCustomer={contact.isClient || contact.status === 'customer'}
          labels={pipelineLabels}
          notes={work.notes}
          onCreateDeal={onCreateDeal}
          onOpenDeal={onOpenDeal}
          tasks={work.tasks}
        />
        <ClientReviewSchedule
          entityType="contact"
          entityId={contact.id}
          isClient={contact.isClient || contact.status === 'customer'}
          canWrite={canWrite}
          users={users}
          onChanged={onReviewChanged}
        />
        {showFoundationCommunications ? (
          <ContactFoundationCommunications
            key={contact.id}
            canWrite={canWrite}
            contact={contact}
            contactId={contact.id}
            onError={onError}
            onSnapshotChange={setFoundationCommunicationsSnapshot}
          />
        ) : null}
        <RecordEmailComposer
          entityType="contact"
          entityId={contact.id}
          canWrite={canWrite}
          recipientOptions={contact.email ? [{ id: contact.id, label: `${fullContactName(contact)} <${contact.email}>` }] : []}
          emptyMessage="Add an email address to this contact before sending email."
          onDeliveryChanged={() => setEmailDeliveryRevision((current) => current + 1)}
        />
        <ContactSequencesCard
          canWrite={canWrite}
          enrollments={outreach.sequenceEnrollments}
          form={outreach.sequenceForm}
          isEnrolling={outreach.isEnrollingSequence}
          onCancel={outreach.handleCancelSequenceEnrollment}
          onEnroll={outreach.handleEnrollSequence}
          onSetForm={outreach.setSequenceForm}
          onToggle={outreach.handleToggleSequences}
          open={outreach.sequencesOpen}
          options={outreach.sequenceOptions}
          status={outreach.sequenceStatus}
        />
        <TouchpointSummary entityType="contact" entityId={contact.id} refreshKey={JSON.stringify({ activities: work.activities, notes: work.notes, tasks: work.tasks, emailDeliveryRevision, foundationCommunicationsSnapshot })} />
        <RecordWorkCards
          activities={work.activities}
          activityMeta={work.activityMeta}
          canWrite={canWrite}
          entityId={contact.id}
          entityType="contact"
          isCreatingNote={work.isCreatingNote}
          isCreatingTask={work.isCreatingTask}
          isLoading={isLoading}
          isLoadingOlderActivities={work.isLoadingOlderActivities}
          isLoadingOlderNotes={work.isLoadingOlderNotes}
          noteBody={work.noteBody}
          notes={work.notes}
          noteMeta={work.noteMeta}
          onCreateNote={work.handleCreateNote}
          onCreateTask={work.handleCreateTask}
          onLoadOlderActivities={work.loadOlderActivities}
          onLoadOlderNotes={work.loadOlderNotes}
          onOpenTasks={onOpenTasks}
          onSetNoteBody={work.setNoteBody}
          onSetTaskForm={work.setTaskForm}
          taskForm={work.taskForm}
          tasks={work.tasks}
          users={users}
        />
      </div>
    </Card>
  )
}

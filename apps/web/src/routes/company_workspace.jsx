import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { RecordEmailComposer } from '../components/record_email_composer'
import { ClientAccountContext } from './client_account_context'
import { ClientReviewSchedule } from './client_review_schedule'
import { CompanyForm } from './company_form'
import { CompanyPeople } from './company_people'
import { createDescription, detailSubtitle, emailRecipientOptions, isIndividualClient } from './company_view'
import { RecordWorkCards } from './record_work'
import { TouchpointSummary } from './touchpoint_summary'

export function CompanyCreateWorkspace({ companyCustomDefinitions, contactCustomDefinitions, contactLookup, form, isSaving, onSetForm, onSubmit }) {
  return (
    <Card>
      <div className="card-stack">
        <div>
          <h2>New client</h2>
          <p>{createDescription(form.clientType)}</p>
        </div>
        <CompanyForm
          contactLookup={contactLookup}
          customDefinitions={isIndividualClient(form.clientType) ? contactCustomDefinitions : companyCustomDefinitions}
          form={form}
          isSubmitting={isSaving}
          onSetForm={onSetForm}
          onSubmit={onSubmit}
          submitLabel="Save client"
        />
      </div>
    </Card>
  )
}

export function CompanyWorkspace({
  canWrite,
  company,
  companyPeople,
  contactLookup,
  companyCustomDefinitions,
  contactCustomDefinitions,
  form,
  isArchiving,
  isLoading,
  isSaving,
  linkedContacts,
  linkedContactDirectory,
  onArchive,
  onCreateDeal,
  onOpenContact,
  onOpenDeal,
  onOpenTasks,
  onReviewChanged,
  onSetForm,
  onUpdate,
  pipelineLabels,
  selectedDeals,
  users,
  work
}) {
  const stableLinkedContacts = linkedContactDirectory.unfilteredContacts
  const emailRecipients = emailRecipientOptions(linkedContactDirectory.knownContacts)

  return (
    <Card>
      <div className="card-stack">
        {isLoading ? <p className="field-hint">Loading client detail...</p> : null}
        <div className="section-header">
          <div>
            <h2>{company.name}</h2>
            <p>{detailSubtitle(company, stableLinkedContacts)}</p>
          </div>
          {canWrite ? (
            <Button className="button-danger" disabled={isArchiving || isSaving} onClick={onArchive}>
              {isArchiving ? 'Archiving…' : 'Archive client'}
            </Button>
          ) : null}
        </div>
        <CompanyForm
          canSubmit={canWrite}
          customDefinitions={companyCustomDefinitions}
          form={form}
          includeLinkedContact={false}
          includeStatus
          isSubmitting={isSaving}
          onSetForm={onSetForm}
          onSubmit={onUpdate}
          submitLabel="Update client"
        />
        <CompanyPeople
          canWrite={canWrite}
          company={company}
          contacts={linkedContacts}
          directory={linkedContactDirectory}
          customDefinitions={contactCustomDefinitions}
          form={companyPeople.form}
          contactLookup={contactLookup}
          onLinkSubmit={companyPeople.handleLinkSubmit}
          onMakePrimary={companyPeople.handleMakePrimary}
          isSaving={companyPeople.isSaving}
          isLinking={companyPeople.isLinking}
          onOpenContact={onOpenContact}
          onSetForm={companyPeople.setForm}
          onSubmit={companyPeople.handleSubmit}
          onToggleLinkForm={companyPeople.handleToggleLinkForm}
          onToggleForm={companyPeople.handleToggleForm}
          onUnlink={companyPeople.handleUnlink}
          linkForm={companyPeople.linkForm}
          onSetLinkForm={companyPeople.setLinkForm}
          showForm={companyPeople.showForm}
          showLinkForm={companyPeople.showLinkForm}
        />
        <RecordEmailComposer
          entityType="company"
          entityId={company.id}
          canWrite={canWrite}
          recipientOptions={emailRecipients}
          emptyMessage="Add a linked person with an email address before sending email from this client."
          mergeFieldHint="Merge fields like {{first_name}}, {{company_name}}, and {{client_status}} are filled in when the email is sent."
        />
        <ClientAccountContext
          canWrite={canWrite}
          contacts={stableLinkedContacts}
          contactTotal={linkedContactDirectory.unfilteredMeta.total}
          deals={selectedDeals}
          isCustomer={company.status === 'customer'}
          labels={pipelineLabels}
          notes={work.notes}
          onCreateDeal={onCreateDeal}
          onOpenContact={onOpenContact}
          onOpenDeal={onOpenDeal}
          tasks={work.tasks}
        />
        <ClientReviewSchedule
          entityType="company"
          entityId={company.id}
          isClient={company.status === 'customer'}
          canWrite={canWrite}
          users={users}
          onChanged={onReviewChanged}
        />
        <TouchpointSummary entityType="company" entityId={company.id} refreshKey={JSON.stringify({ activities: work.activities, notes: work.notes, tasks: work.tasks, linkedContacts: stableLinkedContacts })} />
        <RecordWorkCards
          activities={work.activities}
          activityMeta={work.activityMeta}
          activityAria="Activity list"
          canWrite={canWrite}
          entityId={company.id}
          entityType="company"
          isCreatingNote={work.isCreatingNote}
          isCreatingTask={work.isCreatingTask}
          isLoading={isLoading}
          isLoadingOlderActivities={work.isLoadingOlderActivities}
          isLoadingOlderNotes={work.isLoadingOlderNotes}
          noteBody={work.noteBody}
          notes={work.notes}
          noteMeta={work.noteMeta}
          notesAria="Client notes list"
          onCreateNote={work.handleCreateNote}
          onCreateTask={work.handleCreateTask}
          onLoadOlderActivities={work.loadOlderActivities}
          onLoadOlderNotes={work.loadOlderNotes}
          onOpenTasks={onOpenTasks}
          onSetNoteBody={work.setNoteBody}
          onSetTaskForm={work.setTaskForm}
          taskForm={work.taskForm}
          tasks={work.tasks}
          tasksAria="Client tasks list"
          users={users}
        />
      </div>
    </Card>
  )
}

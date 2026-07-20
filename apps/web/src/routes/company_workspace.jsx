import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'
import { RecordEmailComposer } from '../components/record_email_composer'
import { sendCompanyEmail } from '../lib/companies'
import { ClientAccountContext } from './client_account_context'
import { ClientReviewSchedule } from './client_review_schedule'
import { CompanyForm } from './company_form'
import { CompanyPeople } from './company_people'
import { createDescription, detailSubtitle, emailRecipientOptions, isIndividualClient } from './company_view'
import { RecordWorkCards } from './record_work'
import { TouchpointSummary } from './touchpoint_summary'

export function CompanyCreateWorkspace({ companyCustomDefinitions, contactCustomDefinitions, contacts, form, isSaving, onSetForm, onSubmit }) {
  return (
    <Card>
      <div className="card-stack">
        <div>
          <h2>New client</h2>
          <p>{createDescription(form.clientType)}</p>
        </div>
        <CompanyForm
          contacts={contacts}
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
  companyCustomDefinitions,
  contactCustomDefinitions,
  contactOptions,
  form,
  isArchiving,
  isLoading,
  isSaving,
  linkedContacts,
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
  const emailRecipients = emailRecipientOptions(linkedContacts)

  return (
    <Card>
      <div className="card-stack">
        {isLoading ? <p className="field-hint">Loading client detail...</p> : null}
        <div className="section-header">
          <div>
            <h2>{company.name}</h2>
            <p>{detailSubtitle(company, linkedContacts)}</p>
          </div>
          {canWrite ? (
            <Button className="button-danger" disabled={isArchiving || isSaving} onClick={onArchive}>
              {isArchiving ? 'Archiving…' : 'Archive client'}
            </Button>
          ) : null}
        </div>
        <CompanyForm
          canSubmit={canWrite}
          contacts={contactOptions}
          customDefinitions={companyCustomDefinitions}
          form={form}
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
          customDefinitions={contactCustomDefinitions}
          form={companyPeople.form}
          isSaving={companyPeople.isSaving}
          onOpenContact={onOpenContact}
          onSetForm={companyPeople.setForm}
          onSubmit={companyPeople.handleSubmit}
          onToggleForm={companyPeople.handleToggleForm}
          showForm={companyPeople.showForm}
        />
        <RecordEmailComposer
          entityType="company"
          entityId={company.id}
          canWrite={canWrite}
          recipientOptions={emailRecipients}
          sendEmail={sendCompanyEmail}
          emptyMessage="Add a linked person with an email address before sending email from this client."
          mergeFieldHint="Merge fields like {{first_name}}, {{company_name}}, and {{client_status}} are filled in when the email is sent."
        />
        <ClientAccountContext
          canWrite={canWrite}
          contacts={linkedContacts}
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
        <TouchpointSummary entityType="company" entityId={company.id} refreshKey={JSON.stringify({ activities: work.activities, notes: work.notes, tasks: work.tasks, linkedContacts })} />
        <RecordWorkCards
          activities={work.activities}
          activityAria="Activity list"
          canWrite={canWrite}
          entityId={company.id}
          entityType="company"
          isCreatingNote={work.isCreatingNote}
          isCreatingTask={work.isCreatingTask}
          noteBody={work.noteBody}
          notes={work.notes}
          notesAria="Client notes list"
          onCreateNote={work.handleCreateNote}
          onCreateTask={work.handleCreateTask}
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

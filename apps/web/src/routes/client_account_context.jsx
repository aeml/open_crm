import { Button } from '../components/ui/button'
import { Card } from '../components/ui/card'

function formatMoney(value, currency = 'USD') {
  const amount = Number.parseFloat(value || '0')
  if (!Number.isFinite(amount)) return '$0.00'
  return new Intl.NumberFormat('en-US', { style: 'currency', currency: currency || 'USD' }).format(amount)
}

function dueLabel(value) {
  if (!value) return 'No due date'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? 'Due date unavailable' : `Due ${date.toLocaleDateString()}`
}

function EmptyRow({ children }) {
  return <article className="record-row" role="listitem"><div><p>{children}</p></div></article>
}

function DealRows({ deals, empty, onOpen }) {
  return deals.length === 0 ? <EmptyRow>{empty}</EmptyRow> : deals.map((deal) => (
    <article className="record-row" key={deal.id} role="listitem">
      <div>
        <button className="button button-ghost contact-link" type="button" onClick={() => onOpen(deal.id)}>{deal.name}</button>
        <p>{deal.stageName || deal.status || 'Unstaged'}</p>
      </div>
      <div>
        <p>{formatMoney(deal.valueAmount, deal.valueCurrency)}</p>
        <p>{deal.closeReasonLabel || deal.companyName || (deal.expectedCloseDate ? `Target ${deal.expectedCloseDate}` : 'No client linked')}</p>
      </div>
    </article>
  ))
}

export function ClientAccountContext({
  canWrite,
  contacts = [],
  contactTotal,
  deals = [],
  isCustomer = false,
  labels,
  notes = [],
  onCreateDeal,
  onOpenContact,
  onOpenDeal,
  tasks = []
}) {
  const plural = labels.plural.toLowerCase()
  const wonDeals = deals.filter((deal) => deal.status === 'won')
  const hasAccountContext = isCustomer || wonDeals.length > 0
  const otherDeals = hasAccountContext ? deals.filter((deal) => deal.status !== 'won') : deals
  const visibleWonDeals = wonDeals.slice(0, 5)
  const visibleTasks = tasks.slice(0, 5)
  const visibleNotes = notes.slice(0, 3)
  const visibleContacts = contacts.slice(0, 5)
  const exactContactTotal = Number.isInteger(contactTotal) ? contactTotal : contacts.length

  return (
    <>
      {hasAccountContext ? (
        <Card>
          <div className="card-stack" aria-label="Client account summary">
            <div>
              <h3>Account summary</h3>
              <p className="field-hint">Post-sale context from the existing client, deal, task, note, and contact records.</p>
            </div>
            <div className="button-row" aria-label="Account summary totals">
              <span className="chip">Won {plural}: {wonDeals.length}</span>
              <span className="chip">Open account tasks: {tasks.length}</span>
              {exactContactTotal > 0 ? <span className="chip">Key contacts: {exactContactTotal}</span> : null}
            </div>
            <div>
              <h4>{`Won ${plural}`}</h4>
              <div className="record-list" role="list" aria-label="Won account deals">
                <DealRows deals={visibleWonDeals} empty={`No won ${plural} are linked yet.`} onOpen={onOpenDeal} />
              </div>
            </div>
            <div>
              <h4>Open account tasks</h4>
              <div className="record-list" role="list" aria-label="Open account tasks">
                {visibleTasks.length === 0 ? <EmptyRow>No open tasks on this client record.</EmptyRow> : visibleTasks.map((task) => (
                  <article className="record-row" key={task.id} role="listitem">
                    <div><p>{task.title}</p><p className="field-hint">{task.assignedToUserName || 'Unassigned'}</p></div>
                    <div><p>{dueLabel(task.dueAt)}</p></div>
                  </article>
                ))}
              </div>
            </div>
            <div>
              <h4>Recent account notes</h4>
              <div className="record-list" role="list" aria-label="Recent account notes">
                {visibleNotes.length === 0 ? <EmptyRow>No notes on this client record.</EmptyRow> : visibleNotes.map((note) => (
                  <article className="record-row" key={note.id} role="listitem">
                    <div><p>{note.body}</p><p className="field-hint">{note.createdByUserName || 'Unknown author'}</p></div>
                  </article>
                ))}
              </div>
            </div>
            {contacts.length > 0 ? (
              <div>
                <h4>Key contacts</h4>
                <div className="record-list" role="list" aria-label="Key account contacts">
                  {visibleContacts.map((contact) => (
                    <article className="record-row" key={contact.id} role="listitem">
                      <div>
                        <button className="button button-ghost contact-link" type="button" onClick={() => onOpenContact(contact.id)}>{`${contact.firstName || ''} ${contact.lastName || ''}`.trim()}</button>
                        <p>{contact.relationshipTitle || (contact.isPrimary ? 'Primary contact' : 'Linked contact')}</p>
                      </div>
                      <div><p>{contact.email || contact.phone || 'No contact details'}</p></div>
                    </article>
                  ))}
                </div>
              </div>
            ) : null}
          </div>
        </Card>
      ) : null}
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h3>{`${hasAccountContext ? 'Other related' : 'Related'} ${plural}`}</h3>
              <p>{hasAccountContext ? `Won ${plural} are summarized above; open and lost records remain available here.` : `See active ${plural} tied to this client.`}</p>
            </div>
            {canWrite ? <Button className="button-secondary" onClick={onCreateDeal}>{`Create ${labels.singular}`}</Button> : null}
          </div>
          <div className="record-list" role="list" aria-label="Related deals list">
            <DealRows deals={otherDeals} empty={`No ${hasAccountContext ? 'other ' : ''}related ${plural} yet.`} onOpen={onOpenDeal} />
          </div>
        </div>
      </Card>
    </>
  )
}

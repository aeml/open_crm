import { Card } from '../components/ui/card'
import { Button } from '../components/ui/button'

const contacts = [
  {
    name: 'Morgan Lee',
    role: 'Head of RevOps',
    company: 'Northstar Logistics',
    email: 'morgan@northstar.example',
    status: 'Hot lead'
  },
  {
    name: 'Priya Shah',
    role: 'Procurement Director',
    company: 'Atlas Manufacturing',
    email: 'priya@atlas.example',
    status: 'Follow-up today'
  },
  {
    name: 'Daniel Kim',
    role: 'VP Operations',
    company: 'Bluebird Health',
    email: 'daniel@bluebird.example',
    status: 'Proposal sent'
  }
]

export function ContactsRoute() {
  return (
    <section className="dashboard-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Contacts</h2>
              <p>Keep the right people moving without a bloated CRM mess.</p>
            </div>
            <Button>Add contact</Button>
          </div>
          <div className="record-list" role="list" aria-label="Contacts list">
            {contacts.map((contact) => (
              <article className="record-row" key={contact.email} role="listitem">
                <div>
                  <h3>{contact.name}</h3>
                  <p>{contact.role} · {contact.company}</p>
                </div>
                <div>
                  <p>{contact.email}</p>
                  <p>{contact.status}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
    </section>
  )
}

import { Card } from '../components/ui/card'

const deals = [
  {
    name: 'Northstar expansion',
    stage: 'Proposal',
    amount: '$48,000',
    closeDate: 'Apr 19'
  },
  {
    name: 'Atlas annual contract',
    stage: 'Negotiation',
    amount: '$74,400',
    closeDate: 'Apr 26'
  },
  {
    name: 'Bluebird pilot rollout',
    stage: 'Qualified',
    amount: '$60,000',
    closeDate: 'May 02'
  }
]

export function DealsRoute() {
  return (
    <section className="dashboard-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Deals</h2>
              <p>Pipeline stages stay obvious, not buried in junk UI.</p>
            </div>
          </div>
          <div className="record-list" role="list" aria-label="Deals list">
            {deals.map((deal) => (
              <article className="record-row" key={deal.name} role="listitem">
                <div>
                  <h3>{deal.name}</h3>
                  <p>{deal.stage}</p>
                </div>
                <div>
                  <p>{deal.amount}</p>
                  <p>Close target {deal.closeDate}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
    </section>
  )
}

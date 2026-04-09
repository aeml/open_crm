import { Card } from '../components/ui/card'

const companies = [
  {
    name: 'Northstar Logistics',
    industry: 'Logistics',
    owner: 'Morgan Lee',
    openDeal: '$48,000 expansion'
  },
  {
    name: 'Atlas Manufacturing',
    industry: 'Industrial',
    owner: 'Priya Shah',
    openDeal: '$74,400 annual contract'
  },
  {
    name: 'Bluebird Health',
    industry: 'Healthcare',
    owner: 'Daniel Kim',
    openDeal: '$60,000 pilot rollout'
  }
]

export function CompaniesRoute() {
  return (
    <section className="dashboard-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Companies</h2>
              <p>See account ownership and live pipeline in one place.</p>
            </div>
          </div>
          <div className="record-list" role="list" aria-label="Companies list">
            {companies.map((company) => (
              <article className="record-row" key={company.name} role="listitem">
                <div>
                  <h3>{company.name}</h3>
                  <p>{company.industry}</p>
                </div>
                <div>
                  <p>Owner: {company.owner}</p>
                  <p>{company.openDeal}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
    </section>
  )
}

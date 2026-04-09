import { Card } from '../components/ui/card'

const tasks = [
  {
    title: 'Call Morgan about rollout timing',
    due: 'Today · 11:00 AM',
    owner: 'Owner'
  },
  {
    title: 'Send Atlas redlines',
    due: 'Today · 2:30 PM',
    owner: 'Admin'
  },
  {
    title: 'Prep Bluebird onboarding checklist',
    due: 'Tomorrow · 9:00 AM',
    owner: 'Member'
  }
]

export function TasksRoute() {
  return (
    <section className="dashboard-grid">
      <Card>
        <div className="card-stack">
          <div className="section-header">
            <div>
              <h2>Tasks</h2>
              <p>Next actions stay visible so the team does the work.</p>
            </div>
          </div>
          <div className="record-list" role="list" aria-label="Tasks list">
            {tasks.map((task) => (
              <article className="record-row" key={task.title} role="listitem">
                <div>
                  <h3>{task.title}</h3>
                  <p>{task.due}</p>
                </div>
                <div>
                  <p>{task.owner}</p>
                </div>
              </article>
            ))}
          </div>
        </div>
      </Card>
    </section>
  )
}

export function PageHeader({ eyebrow, title, description, actions }) {
  return (
    <section className="page-header">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h2>{title}</h2>
        <p className="page-description">{description}</p>
      </div>
      <div>{actions}</div>
    </section>
  )
}

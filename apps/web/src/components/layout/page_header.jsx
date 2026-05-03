export function PageHeader({ eyebrow, title, description, actions }) {
  return (
    <section className="page-header" aria-labelledby="page-heading">
      <div>
        <p className="eyebrow">{eyebrow}</p>
        <h1 id="page-heading">{title}</h1>
        <p className="page-description">{description}</p>
      </div>
      {actions ? <div>{actions}</div> : null}
    </section>
  )
}

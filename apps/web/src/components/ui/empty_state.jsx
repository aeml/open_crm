import { Button } from './button'

export function EmptyState({ title, description, actionLabel, onAction }) {
  return (
    <article className="empty-state" role="listitem">
      <div>
        <p>{title}</p>
        {description ? <p className="field-hint">{description}</p> : null}
      </div>
      {actionLabel && onAction ? (
        <div>
          <Button className="button-secondary" type="button" onClick={onAction}>{actionLabel}</Button>
        </div>
      ) : null}
    </article>
  )
}

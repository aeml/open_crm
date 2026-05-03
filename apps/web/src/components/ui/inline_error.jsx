import { Button } from './button'

export function InlineError({ message, onRetry, retryLabel = 'Retry' }) {
  return (
    <div className="card-stack" role="alert">
      <p className="form-error">{message}</p>
      {onRetry ? (
        <div>
          <Button className="button-secondary" type="button" onClick={onRetry}>
            {retryLabel}
          </Button>
        </div>
      ) : null}
    </div>
  )
}

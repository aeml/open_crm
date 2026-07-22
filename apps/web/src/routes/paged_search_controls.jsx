import { Button } from '../components/ui/button'
import { InlineError } from '../components/ui/inline_error'

export function PagedSearchControls({ hint, id, label, lookup, placeholder }) {
  const hintID = `${id}-hint`
  return (
    <div className="field">
      <label className="field-label" htmlFor={id}>{label}</label>
      <div className="button-row">
        <input
          aria-describedby={hintID}
          className="text-input"
          id={id}
          onChange={(event) => lookup.setQuery(event.target.value)}
          placeholder={placeholder}
          value={lookup.query}
        />
        <Button className="button-secondary" type="button" onClick={lookup.search} disabled={lookup.isLoading}>
          {lookup.isLoading ? 'Searching…' : 'Search'}
        </Button>
        {lookup.appliedQuery ? <Button className="button-ghost" type="button" onClick={lookup.reset} disabled={lookup.isLoading}>Clear</Button> : null}
      </div>
      <span className="field-hint" id={hintID}>{hint}</span>
      {lookup.error ? <InlineError message={lookup.error} /> : null}
    </div>
  )
}

import { useEffect, useMemo, useState } from 'react'
import { customFieldOperators } from '../../lib/custom_fields'
import { Button } from './button'
import { Field } from './field'

const emptyFilter = { fieldKey: '', operator: '', value: '' }

function FilterValue({ definition, value, onChange }) {
  if (definition?.dataType === 'select') {
    return <select className="text-input" value={value} onChange={onChange} required><option value="">Choose a value</option>{definition.options.map((option) => <option key={option} value={option}>{option}</option>)}</select>
  }
  if (definition?.dataType === 'boolean') {
    return <select className="text-input" value={value} onChange={onChange} required><option value="">Choose a value</option><option value="true">Yes</option><option value="false">No</option></select>
  }
  const type = definition?.dataType === 'number' ? 'number' : definition?.dataType === 'date' ? 'date' : 'text'
  return <input className="text-input" type={type} step={type === 'number' ? 'any' : undefined} value={value} onChange={onChange} required />
}

export function CustomFieldFilter({ definitions = [], value = emptyFilter, onApply, onClear }) {
  const [draft, setDraft] = useState(value)
  const definition = useMemo(() => definitions.find((item) => item.fieldKey === draft.fieldKey), [definitions, draft.fieldKey])
  const operators = customFieldOperators(definition?.dataType)

  useEffect(() => setDraft(value), [value.fieldKey, value.operator, value.value])

  if (definitions.length === 0) return null
  return (
    <form className="custom-field-filter" onSubmit={(event) => { event.preventDefault(); onApply(draft) }}>
      <Field label="Custom field filter">
        <select className="text-input" value={draft.fieldKey} onChange={(event) => {
          const nextDefinition = definitions.find((item) => item.fieldKey === event.target.value)
          setDraft({ fieldKey: event.target.value, operator: customFieldOperators(nextDefinition?.dataType)[0]?.value || '', value: '' })
        }} required>
          <option value="">Choose a field</option>
          {definitions.map((item) => <option key={item.id} value={item.fieldKey}>{item.label}</option>)}
        </select>
      </Field>
      <Field label="Condition">
        <select className="text-input" value={draft.operator} onChange={(event) => setDraft((current) => ({ ...current, operator: event.target.value }))} disabled={!definition} required>
          {operators.map((operator) => <option key={operator.value} value={operator.value}>{operator.label}</option>)}
        </select>
      </Field>
      <Field label="Value"><FilterValue definition={definition} value={draft.value} onChange={(event) => setDraft((current) => ({ ...current, value: event.target.value }))} /></Field>
      <div className="button-row">
        <Button type="submit" disabled={!definition}>Apply custom filter</Button>
        {value.fieldKey ? <Button className="button-secondary" type="button" onClick={() => { setDraft(emptyFilter); onClear() }}>Clear custom filter</Button> : null}
      </div>
    </form>
  )
}

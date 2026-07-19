import { Field } from './field'

function CustomFieldInput({ definition, value, onChange }) {
  const common = { className: 'text-input', value: value ?? '', onChange: (event) => onChange(event.target.value), required: definition.required }
  if (definition.dataType === 'select') {
    return <select {...common}><option value="">Not set</option>{definition.options.map((option) => <option key={option} value={option}>{option}</option>)}</select>
  }
  if (definition.dataType === 'boolean') {
    return <select {...common}><option value="">Not set</option><option value="true">Yes</option><option value="false">No</option></select>
  }
  const type = definition.dataType === 'number' ? 'number' : definition.dataType === 'date' ? 'date' : 'text'
  return <input {...common} type={type} step={definition.dataType === 'number' ? 'any' : undefined} maxLength={definition.dataType === 'text' ? 500 : undefined} />
}

export function CustomFieldsForm({ definitions = [], values = {}, onChange }) {
  if (definitions.length === 0) return null
  return (
    <fieldset className="card-stack custom-fields-group">
      <legend>Custom fields</legend>
      {definitions.map((definition) => (
        <Field key={definition.id} label={`${definition.label}${definition.required ? ' (required)' : ''}`} hint={`Stable import/export key: custom:${definition.fieldKey}`}>
          <CustomFieldInput definition={definition} value={values[definition.fieldKey]} onChange={(value) => onChange({ ...values, [definition.fieldKey]: value })} />
        </Field>
      ))}
    </fieldset>
  )
}

export function CustomFieldValue({ definition, value }) {
  if (value === undefined || value === null || value === '') return null
  const display = definition.dataType === 'boolean' ? (value ? 'Yes' : 'No') : String(value)
  return <span><strong>{definition.label}:</strong> {display}</span>
}

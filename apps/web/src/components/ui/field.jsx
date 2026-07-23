export function Field({ label, children, hint }) {
  return (
    <label className="field">
      <span className="field-label">{label}</span>
      {children}
      {hint ? <span className="field-hint">{hint}</span> : null}
    </label>
  )
}

export function ControlledTextField({ form, hint, label, multiline = false, name, setForm, ...props }) {
  const controlProps = {
    ...props,
    className: 'text-input',
    value: form[name],
    onChange: (event) => setForm((current) => ({ ...current, [name]: event.target.value }))
  }
  return (
    <Field label={label} hint={hint}>
      {multiline ? <textarea {...controlProps} /> : <input {...controlProps} />}
    </Field>
  )
}

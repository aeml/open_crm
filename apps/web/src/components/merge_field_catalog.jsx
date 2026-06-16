export function MergeFieldCatalog({ groups = [], compact = false }) {
  if (!groups.length) {
    return null
  }

  return (
    <div className={compact ? 'merge-field-catalog merge-field-catalog-compact' : 'merge-field-catalog'} aria-label="Available merge fields">
      {groups.map((group) => (
        <section className="merge-field-group" key={group.key || group.label}>
          <h4>{group.label}</h4>
          <div className="merge-field-token-list">
            {(group.fields || []).map((field) => (
              <code className="merge-field-token" key={`${group.key || group.label}-${field.token}`} title={field.description || field.label}>
                {field.token}
              </code>
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}

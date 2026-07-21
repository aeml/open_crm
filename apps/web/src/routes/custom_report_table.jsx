function displayValue(value) {
  if (value === null || value === undefined || value === '') return '—'
  return String(value)
}

export function CustomReportTable({ definition, result, label = `${definition.name} results`, caption = `${definition.name} · page ${result.page}` }) {
  return (
    <div className="table-scroll" tabIndex="0" role="region" aria-label={label}>
      <table className="data-table">
        <caption>{caption}</caption>
        <thead>
          <tr>{result.columns.map((column) => <th scope="col" key={column.key}>{column.label}</th>)}</tr>
        </thead>
        <tbody>
          {result.rows.length === 0 ? (
            <tr><td colSpan={Math.max(1, result.columns.length)}>No records match this saved report.</td></tr>
          ) : result.rows.map((row, rowIndex) => (
            <tr key={`${result.page}-${rowIndex}`}>
              {result.columns.map((column) => <td key={column.key}>{displayValue(row.values?.[column.key])}</td>)}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

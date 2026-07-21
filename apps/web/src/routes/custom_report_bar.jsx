import { CustomReportTable } from './custom_report_table'

function chartRows(result) {
  if (result.columns.length !== 2 || !['integer', 'numeric'].includes(result.columns[1].dataType)) return null
  const rows = result.rows.map((row) => ({
    category: row.values?.[result.columns[0].key] || '—',
    label: row.values?.[result.columns[1].key] || '0',
    value: Number(row.values?.[result.columns[1].key] || 0)
  }))
  return rows.every((row) => Number.isFinite(row.value) && row.value >= 0) ? rows : null
}

export function CustomReportBar({ definition, result }) {
  const rows = chartRows(result)
  if (!rows) return <p role="alert">This report returned values that cannot be shown safely as a grouped bar chart.</p>
  const maximum = Math.max(1, ...rows.map((row) => row.value))
  return (
    <>
      {rows.length ? (
        <div className="report-bar-chart" role="img" aria-label={`${definition.name} grouped bar chart. Exact values follow in a data table.`}>
          {rows.map((row, index) => (
            <div className="report-bar-row" key={`${row.category}-${index}`}>
              <span>{row.category}</span>
              <span className="report-bar-track"><span style={{ width: `${(row.value / maximum) * 100}%` }} /></span>
              <strong>{row.label}</strong>
            </div>
          ))}
        </div>
      ) : null}
      <CustomReportTable definition={definition} result={result} label={`${definition.name} chart data`} caption={`${definition.name} · grouped bar data · page ${result.page}`} />
    </>
  )
}

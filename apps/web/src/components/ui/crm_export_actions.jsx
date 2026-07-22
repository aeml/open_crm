export function CRMExportActions({ canExport, directURL, durableURL }) {
  if (!canExport) return null
  return (
    <>
      <a className="button button-secondary" href={directURL}>Export CSV</a>
      <a className="button button-secondary" href={durableURL}>Queue large CSV</a>
    </>
  )
}

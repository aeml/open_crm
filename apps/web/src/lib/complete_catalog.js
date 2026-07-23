export async function loadCompleteCatalog(loadPage, field, changedMessage, incompleteMessage) {
  const rowsById = new Map()
  let expectedTotal = null
  for (let page = 1; page <= 501; page += 1) {
    const result = await loadPage({ page, pageSize: 100 })
    const total = Number(result.meta?.total)
    if (!Number.isSafeInteger(total) || total < 0 || (expectedTotal !== null && total !== expectedTotal)) {
      throw new Error(changedMessage)
    }
    expectedTotal = total
    result[field].forEach((row) => rowsById.set(row.id, row))
    if (rowsById.size >= expectedTotal) return [...rowsById.values()]
    if (result[field].length === 0) break
  }
  throw new Error(incompleteMessage)
}

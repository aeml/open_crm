export function activeRunRefreshDelay(runs, now = Date.now()) {
  let nextDelay = null
  for (const run of runs) {
    if (run.status === 'running') return 1000
    if (run.status !== 'queued') continue
    const scheduledAt = new Date(run.scheduledAt || '').getTime()
    if (!Number.isFinite(scheduledAt) || scheduledAt <= now) return 1000
    nextDelay = Math.min(nextDelay ?? 60000, Math.max(1000, scheduledAt - now), 60000)
  }
  return nextDelay
}

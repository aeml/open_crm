import { existsSync, readFileSync, readdirSync, statSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { gzipSync } from 'node:zlib'

const scriptDirectory = fileURLToPath(new URL('.', import.meta.url))
const distDirectory = process.env.VITE_OUT_DIR
  ? resolve(process.env.VITE_OUT_DIR)
  : resolve(scriptDirectory, '..', 'dist')
const indexPath = resolve(distDirectory, 'index.html')
const assetsDirectory = resolve(distDirectory, 'assets')

if (!existsSync(indexPath) || !existsSync(assetsDirectory)) {
  throw new Error(`bundle output is missing from ${distDirectory}; run npm run build before checking budgets`)
}

const kibibytes = (value) => Math.round((value / 1024) * 100) / 100
const describe = (bytes) => `${kibibytes(bytes)} KiB`
const failures = []

function enforce(label, actual, maximum) {
  if (actual > maximum) {
    failures.push(`${label}: ${describe(actual)} exceeds ${describe(maximum)}`)
  }
}

function assetSize(name) {
  const path = resolve(assetsDirectory, name)
  const contents = readFileSync(path)
  return {
    name,
    raw: statSync(path).size,
    gzip: gzipSync(contents, { level: 9 }).length,
  }
}

const indexHTML = readFileSync(indexPath, 'utf8')
const entryMatch = indexHTML.match(/<script[^>]+src="\/assets\/([^"?]+\.js)"/i)
if (!entryMatch) {
  throw new Error(`could not identify the JavaScript entry chunk in ${indexPath}`)
}

const javascript = readdirSync(assetsDirectory)
  .filter((name) => name.endsWith('.js'))
  .map(assetSize)
const stylesheets = readdirSync(assetsDirectory)
  .filter((name) => name.endsWith('.css'))
  .map(assetSize)
const entry = javascript.find(({ name }) => name === entryMatch[1])
if (!entry) {
  throw new Error(`entry chunk ${entryMatch[1]} is missing from ${assetsDirectory}`)
}

const asynchronousChunks = javascript.filter(({ name }) => name !== entry.name)
for (const hiddenFoundation of [
  'settings_calendar-',
  'settings_lead_audiences-',
  'settings_lead_scoring-',
  'settings_marketing_email_campaigns-',
  'settings_nurture_campaigns-',
]) {
  if (javascript.some(({ name }) => name.startsWith(hiddenFoundation))) {
    failures.push(`incomplete production route was bundled: ${hiddenFoundation}`)
  }
}
const javascriptContents = javascript.map(({ name }) => ({
  name,
  contents: readFileSync(resolve(assetsDirectory, name), 'utf8'),
}))
for (const hiddenFoundationUI of [
  'Start an outbound call and log the outcome on this contact.',
  'Send compliant one-to-one texts and log inbound replies.',
  'Schedule meetings and keep a contact-level calendar history.',
  '/api/calls',
  '/api/sms-messages',
  '/api/calendar-events',
  '/api/calendar-booking-links',
]) {
  const containingAsset = javascriptContents.find(({ contents }) => contents.includes(hiddenFoundationUI))
  if (containingAsset) {
    failures.push(`incomplete production communication UI was bundled in ${containingAsset.name}: ${hiddenFoundationUI}`)
  }
}
const largestAsynchronous = asynchronousChunks.reduce(
  (largest, chunk) => (chunk.raw > largest.raw ? chunk : largest),
  { name: 'none', raw: 0, gzip: 0 },
)
const totals = [...javascript, ...stylesheets].reduce(
  (sum, asset) => ({ raw: sum.raw + asset.raw, gzip: sum.gzip + asset.gzip }),
  { raw: 0, gzip: 0 },
)
const cssTotals = stylesheets.reduce(
  (sum, asset) => ({ raw: sum.raw + asset.raw, gzip: sum.gzip + asset.gzip }),
  { raw: 0, gzip: 0 },
)

enforce('entry chunk raw', entry.raw, 190 * 1024)
enforce('entry chunk gzip', entry.gzip, 65 * 1024)
for (const chunk of asynchronousChunks) {
  enforce(`async chunk ${chunk.name} raw`, chunk.raw, 60 * 1024)
  enforce(`async chunk ${chunk.name} gzip`, chunk.gzip, 16 * 1024)
}
enforce('all JavaScript and CSS raw', totals.raw, 795 * 1024)
enforce('all JavaScript and CSS gzip', totals.gzip, 247 * 1024)
enforce('all CSS raw', cssTotals.raw, 20 * 1024)
enforce('all CSS gzip', cssTotals.gzip, 5 * 1024)

console.log(
  [
    `bundle_budget entry=${entry.name} raw=${describe(entry.raw)} gzip=${describe(entry.gzip)}`,
    `largest_async=${largestAsynchronous.name} raw=${describe(largestAsynchronous.raw)} gzip=${describe(largestAsynchronous.gzip)}`,
    `total_assets raw=${describe(totals.raw)} gzip=${describe(totals.gzip)}`,
  ].join(' '),
)

if (failures.length > 0) {
  throw new Error(`bundle budget failed:\n- ${failures.join('\n- ')}`)
}

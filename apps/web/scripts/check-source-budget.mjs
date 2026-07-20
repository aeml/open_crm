import { readFileSync, readdirSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDirectory = fileURLToPath(new URL('.', import.meta.url))
const routesDirectory = resolve(scriptDirectory, '..', 'src', 'routes')
const defaultMaximum = 500
const maximums = new Map([
  ['contacts.jsx', 650],
  ['companies.jsx', 775],
  ['deals.jsx', 775],
  ['tasks.jsx', 750],
  ['settings_automations.jsx', 700],
  ['dashboard.jsx', 550],
])

function lineCount(path) {
  const source = readFileSync(path, 'utf8')
  if (source.length === 0) {
    return 0
  }
  return source.split(/\r?\n/).length - (source.endsWith('\n') ? 1 : 0)
}

const measurements = readdirSync(routesDirectory)
  .filter((name) => /\.(js|jsx)$/.test(name) && !name.includes('.test.'))
  .map((name) => ({ name, lines: lineCount(resolve(routesDirectory, name)), maximum: maximums.get(name) ?? defaultMaximum }))
  .sort((left, right) => right.lines - left.lines)
const failures = measurements.filter(({ lines, maximum }) => lines > maximum)

console.log(
  `source_budget ${measurements.slice(0, 8).map(({ name, lines, maximum }) => `${name}=${lines}/${maximum}`).join(' ')}`,
)

if (failures.length > 0) {
  throw new Error(`source-size ratchet failed:\n- ${failures.map(({ name, lines, maximum }) => `${name}: ${lines} lines exceeds ${maximum}`).join('\n- ')}`)
}

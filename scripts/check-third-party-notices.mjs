import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { existsSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { dirname, join, relative, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const scriptDirectory = dirname(fileURLToPath(import.meta.url))
const repositoryRoot = resolve(scriptDirectory, '..')
const apiRoot = join(repositoryRoot, 'apps', 'api')
const webRoot = join(repositoryRoot, 'apps', 'web')
const noticesPath = join(repositoryRoot, 'THIRD_PARTY_NOTICES.md')
const policy = JSON.parse(readFileSync(join(scriptDirectory, 'license-policy.json'), 'utf8'))
const allowedLicenses = new Set(policy.allowedSpdxLicenses)
const npmBuildOutputPackages = new Set(policy.npmBuildOutputPackages)
const mainGoModule = 'github.com/aeml/open_crm/apps/api'
const supportedCommands = ['./cmd/open_crm_api', './cmd/migrate', './cmd/seed']

function fail(message) {
  throw new Error(message)
}

function command(commandName, args, options = {}) {
  return execFileSync(commandName, args, {
    cwd: options.cwd || repositoryRoot,
    encoding: 'utf8',
    env: { ...process.env, GOTOOLCHAIN: policy.goToolchainVersion }
  }).trim()
}

function normalizeText(value) {
  return String(value || '')
    .replace(/\r\n/g, '\n')
    .replace(/[ \t]+$/gm, '')
    .trim()
}

function markdownCell(value) {
  return String(value).replaceAll('|', '\\|')
}

function licenseIdentifiers(expression) {
  return String(expression || '')
    .replace(/[()]/g, ' ')
    .split(/\s+/)
    .filter((token) => token && token !== 'AND' && token !== 'OR' && token !== 'WITH')
}

function validateLicense(component, expression) {
  const identifiers = licenseIdentifiers(expression)
  if (identifiers.length === 0) {
    fail(`${component} does not declare a license`)
  }
  for (const identifier of identifiers) {
    if (!allowedLicenses.has(identifier)) {
      fail(`${component} uses unreviewed license ${identifier}; review obligations and update scripts/license-policy.json explicitly`)
    }
  }
}

function validateEnvironment() {
  if (process.platform !== 'linux' || Number(process.versions.node.split('.')[0]) !== 24) {
    fail(`license inventory must run on supported Linux/Node 24; found ${process.platform}/Node ${process.versions.node}`)
  }
}

function findLegalFiles(directory, requireLicense = true) {
  const entries = readdirSync(directory, { withFileTypes: true })
    .filter((entry) => entry.isFile())
    .map((entry) => entry.name)
  const licenseName = entries.sort().find((name) => /^(license|licence|copying)(\.|$)/i.test(name))
  if (!licenseName && requireLicense) {
    fail(`no license text found in ${relative(repositoryRoot, directory)}`)
  }
  const noticeNames = entries.filter((name) => /^(notice|patents)(\.|$)/i.test(name)).sort()
  return [licenseName, ...noticeNames]
    .filter(Boolean)
    .map((name) => `${name}\n\n${normalizeText(readFileSync(join(directory, name), 'utf8'))}`)
    .join('\n\n')
}

function goRuntimeComponents() {
  const lines = command('go', [
    'list',
    '-buildvcs=false',
    '-deps',
    '-f',
    '{{with .Module}}{{.Path}}\t{{.Version}}\t{{.Dir}}{{end}}',
    ...supportedCommands
  ], { cwd: apiRoot }).split('\n').filter(Boolean)
  const modules = new Map()
  for (const line of lines) {
    const [modulePath, version, directory] = line.split('\t')
    if (!modulePath || modulePath === mainGoModule) continue
    modules.set(`${modulePath}@${version}`, { modulePath, version, directory })
  }

  const approvedModules = new Set(Object.keys(policy.goRuntimeModules))
  const components = [...modules.values()].sort((left, right) => left.modulePath.localeCompare(right.modulePath)).map((module) => {
    const license = policy.goRuntimeModules[module.modulePath]
    if (!license) fail(`Go runtime module ${module.modulePath} is not reviewed in scripts/license-policy.json`)
    validateLicense(`${module.modulePath}@${module.version}`, license)
    if (!module.directory || !existsSync(module.directory)) fail(`Go runtime module directory is unavailable for ${module.modulePath}@${module.version}`)
    approvedModules.delete(module.modulePath)
    return {
      ecosystem: 'Go',
      name: module.modulePath,
      version: module.version,
      license,
      source: `https://pkg.go.dev/${module.modulePath}@${module.version}`,
      legalText: findLegalFiles(module.directory)
    }
  })
  if (approvedModules.size > 0) {
    fail(`reviewed Go modules are no longer in the shipped command graph: ${[...approvedModules].sort().join(', ')}`)
  }

  const [goRoot, goVersion] = command('go', ['env', 'GOROOT', 'GOVERSION']).split('\n')
  validateLicense(goVersion, policy.goToolchainLicense)
  components.unshift({
    ecosystem: 'Go',
    name: 'Go standard library',
    version: goVersion,
    license: policy.goToolchainLicense,
    source: 'https://go.dev/LICENSE',
    legalText: findLegalFiles(goRoot)
  })
  return components
}

function npmComponents() {
  const lock = JSON.parse(readFileSync(join(webRoot, 'package-lock.json'), 'utf8'))
  const components = new Map()
  for (const [packagePath, lockEntry] of Object.entries(lock.packages || {})) {
    if (!packagePath.startsWith('node_modules/')) continue
    const directory = join(webRoot, packagePath)
    const manifestPath = join(directory, 'package.json')
    if (!existsSync(manifestPath)) {
      if (lockEntry.optional) continue
      fail(`npm dependency ${packagePath} is not installed; run npm ci before checking notices`)
    }
    const manifest = JSON.parse(readFileSync(manifestPath, 'utf8'))
    const name = String(manifest.name || packagePath.replace(/^.*node_modules\//, ''))
    const version = String(manifest.version || lockEntry.version || '')
    if (!version || (lockEntry.version && version !== lockEntry.version)) {
      fail(`npm dependency ${name} has installed version ${version || '(missing)'} but the lockfile requires ${lockEntry.version || '(missing)'}`)
    }
    const license = typeof manifest.license === 'string' ? manifest.license : String(lockEntry.license || '')
    const componentName = `${name}@${version}`
    validateLicense(componentName, license)
    const runtime = lockEntry.dev !== true
    const buildOutput = npmBuildOutputPackages.has(name)
    const existing = components.get(componentName)
    if (existing) {
      existing.runtime ||= runtime
      if ((runtime || buildOutput) && !existing.legalText) existing.legalText = findLegalFiles(directory)
      continue
    }
    components.set(componentName, {
      ecosystem: 'npm',
      name,
      version,
      license,
      source: `https://www.npmjs.com/package/${name}/v/${version}`,
      runtime,
      buildOutput,
      legalText: runtime || buildOutput ? findLegalFiles(directory) : ''
    })
  }
  const missingBuildOutputPackages = [...npmBuildOutputPackages].filter((name) => ![...components.values()].some((component) => component.name === name))
  if (missingBuildOutputPackages.length > 0) {
    fail(`reviewed npm build-output packages are no longer installed: ${missingBuildOutputPackages.sort().join(', ')}`)
  }
  return [...components.values()].sort((left, right) => left.name.localeCompare(right.name) || left.version.localeCompare(right.version))
}

function licenseSummary(components) {
  const counts = new Map()
  for (const component of components) counts.set(component.license, (counts.get(component.license) || 0) + 1)
  return [...counts.entries()].sort((left, right) => left[0].localeCompare(right[0]))
}

function renderTable(components) {
  return [
    '| Component | Version | License | Source |',
    '| --- | --- | --- | --- |',
    ...components.map((component) => `| ${markdownCell(component.name)} | ${markdownCell(component.version)} | ${markdownCell(component.license)} | [source](${component.source}) |`)
  ].join('\n')
}

function renderDistributedNpmTable(components) {
  return [
    '| Component | Version | License | Inclusion | Source |',
    '| --- | --- | --- | --- | --- |',
    ...components.map((component) => `| ${markdownCell(component.name)} | ${markdownCell(component.version)} | ${markdownCell(component.license)} | ${component.runtime ? 'runtime dependency' : 'build-output contributor'} | [source](${component.source}) |`)
  ].join('\n')
}

function fencedLegalText(value) {
  const longestRun = Math.max(0, ...(value.match(/`+/g) || []).map((match) => match.length))
  const fence = '`'.repeat(Math.max(3, longestRun + 1))
  return [`${fence}text`, value, fence]
}

function renderNotices(goComponents, npmAllComponents) {
  const npmRuntimeComponents = npmAllComponents.filter((component) => component.runtime)
  const npmDistributedComponents = npmAllComponents.filter((component) => component.runtime || component.buildOutput)
  const runtimeComponents = [...goComponents, ...npmDistributedComponents]
  const developmentComponents = npmAllComponents.filter((component) => !component.runtime)
  const legalGroups = new Map()
  for (const component of runtimeComponents) {
    const digest = createHash('sha256').update(component.legalText).digest('hex')
    const group = legalGroups.get(digest) || { text: component.legalText, components: [] }
    group.components.push(`${component.name}@${component.version}`)
    legalGroups.set(digest, group)
  }

  const lines = [
    '# Third-Party Notices',
    '',
    'This file is generated by `scripts/check-third-party-notices.mjs` from the supported production command graph and the installed Node 24 dependency graph. Do not edit it by hand.',
    '',
    'Open CRM itself is distributed under the MIT license in `LICENSE`. The production API binaries and browser bundle include the runtime components below. Vite and Rollup are conservatively included because their helpers or polyfills may contribute to generated browser output.',
    '',
    '## Distributed Go components',
    '',
    renderTable(goComponents),
    '',
    '## Distributed npm components',
    '',
    renderDistributedNpmTable(npmDistributedComponents),
    '',
    '## Development dependency review summary',
    '',
    `The supported Linux/Node 24 install contains ${developmentComponents.length} unique development-only npm packages. CI rejects a missing, malformed, or unreviewed license declaration. Build-output contributors listed above also appear here because npm classifies them as development dependencies.`,
    '',
    renderTable(developmentComponents),
    '',
    '### License summary',
    '',
    '| License expression | Packages |',
    '| --- | ---: |',
    ...licenseSummary(developmentComponents).map(([license, count]) => `| ${markdownCell(license)} | ${count} |`),
    '',
    '## Required license and notice texts',
    ''
  ]

  for (const group of [...legalGroups.values()].sort((left, right) => left.components[0].localeCompare(right.components[0]))) {
    lines.push(`### ${group.components.sort().join(', ')}`, '', ...fencedLegalText(group.text), '')
  }
  return `${lines.join('\n').trim()}\n`
}

validateEnvironment()
const goComponents = goRuntimeComponents()
const npmAllComponents = npmComponents()
const generated = renderNotices(goComponents, npmAllComponents)
if (process.argv.includes('--write')) {
  writeFileSync(noticesPath, generated)
  process.stdout.write(`updated ${relative(repositoryRoot, noticesPath)}\n`)
} else if (process.argv.includes('--check')) {
  if (!existsSync(noticesPath) || readFileSync(noticesPath, 'utf8') !== generated) {
    fail('THIRD_PARTY_NOTICES.md is stale; run node scripts/check-third-party-notices.mjs --write after npm ci')
  }
  process.stdout.write(`license_policy go_runtime=${goComponents.length} npm_runtime=${npmAllComponents.filter((component) => component.runtime).length} npm_build_output=${npmAllComponents.filter((component) => component.buildOutput && !component.runtime).length} npm_development=${npmAllComponents.filter((component) => !component.runtime).length}\n`)
} else {
  process.stdout.write(generated)
}

import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

describe('favicon asset', () => {
  it('ships a public favicon and references it from the HTML shell', () => {
    const publicFaviconPath = resolve(process.cwd(), 'public', 'favicon.svg')
    const indexHtmlPath = resolve(process.cwd(), 'index.html')

    expect(existsSync(publicFaviconPath)).toBe(true)
    expect(readFileSync(indexHtmlPath, 'utf8')).toContain('href="/favicon.svg"')
  })
})

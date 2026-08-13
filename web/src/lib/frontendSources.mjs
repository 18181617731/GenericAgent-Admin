import { readFileSync, readdirSync } from 'node:fs'
import { join, relative, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

const SRC_DIR = fileURLToPath(new URL('../', import.meta.url))

const walk = (dir) => readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
  const path = join(dir, entry.name)
  if (entry.isDirectory()) return walk(path)
  if (!/\.(jsx|js|mjs)$/.test(entry.name)) return []
  if (/\.test\.(jsx|js|mjs)$/.test(entry.name)) return []
  return [path]
})

// Contract tests scan behaviour, not file layout: they read every frontend
// source so moving a guard between modules can never silently drop coverage.
export const frontendSources = () => walk(SRC_DIR).map(path => ({
  file: relative(SRC_DIR, path).split(sep).join('/'),
  source: readFileSync(path, 'utf8'),
}))

export const frontendSource = () => frontendSources().map(entry => entry.source).join('\n')

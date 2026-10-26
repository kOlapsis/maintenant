// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Acceptance tests of 047-audit-small-fixes, C13: braces (GHSA-vfj7-8cjw-p6xm) leaves the tree with @vue/eslint-config-typescript.

// @vitest-environment node

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { ESLint } from 'eslint'
import { describe, it, expect } from 'vitest'

const root = resolve(__dirname, '..')
const wrapper = '@vue/eslint-config-typescript'

function readJSON<T>(file: string): T {
  return JSON.parse(readFileSync(resolve(root, file), 'utf8')) as T
}

type Deps = Record<string, string> | undefined
type Pkg = { overrides?: Deps; dependencies?: Deps; devDependencies?: Deps; optionalDependencies?: Deps }

describe('braces (GHSA-vfj7-8cjw-p6xm)', () => {
  // FR-008, US3 scenario 4: package.json neither pins braces nor lists the package that pulled it.
  it('is neither overridden nor pulled in by package.json', () => {
    const pkg = readJSON<Pkg>('package.json')
    expect(pkg.overrides?.braces, 'overrides.braces must be gone').toBeUndefined()
    for (const block of ['dependencies', 'devDependencies', 'optionalDependencies'] as const) {
      expect(pkg[block]?.[wrapper], `${block} still lists ${wrapper}`).toBeUndefined()
    }
  })

  // FR-008, US3 scenario 4: nothing in the lock installs braces or its wrapper, at any depth.
  it('is absent from package-lock.json', () => {
    const lock = readJSON<{ packages: Record<string, unknown> }>('package-lock.json')
    const paths = Object.keys(lock.packages)
    expect(paths.length).toBeGreaterThan(1)
    const offenders = paths.filter((p) => /(^|\/)node_modules\/(braces|@vue\/eslint-config-typescript)$/.test(p))
    expect(offenders).toEqual([])
  })
})

describe('eslint wired on typescript-eslint and vue-eslint-parser', () => {
  // FR-008: the same rules apply without the wrapper, on .ts and inside <script setup lang="ts">.
  const linter = new ESLint({ cwd: root })

  it.each([
    ['src/sample.ts', 'export const n: any = 1\n'],
    ['src/Sample.vue', '<script setup lang="ts">\nconst n: any = 1\nconsole.log(n)\n</script>\n<template><div>{{ n }}</div></template>\n'],
    ['src/components/Other.vue', '<script setup lang="ts">\nfunction f(x: any) {\n  return x\n}\nconsole.log(f(1))\n</script>\n<template><span /></template>\n'],
  ])(
    'reports @typescript-eslint/no-explicit-any in %s',
    async (file, code) => {
      const results = await linter.lintText(code, { filePath: resolve(root, file) })
      expect(results).toHaveLength(1)
      const messages = results[0]!.messages
      expect(messages.filter((m) => m.fatal), 'parse error').toEqual([])
      expect(messages.map((m) => m.ruleId)).toContain('@typescript-eslint/no-explicit-any')
    },
    30_000,
  )
})

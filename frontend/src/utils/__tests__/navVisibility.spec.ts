// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from 'vitest'
import { isNavItemVisible, type NavItem } from '../navVisibility'

function ctx(overrides: Partial<Parameters<typeof isNavItemVisible>[1]> = {}) {
  return {
    hasFeature: () => true,
    availableRuntimes: ['docker'],
    ...overrides,
  }
}

describe('isNavItemVisible', () => {
  it('shows a plain item with no restrictions', () => {
    const item: NavItem = { type: 'item', to: '/dashboard', label: 'Dashboard' }
    expect(isNavItemVisible(item, ctx())).toBe(true)
  })

  it('hides a feature-gated item when the feature is not available', () => {
    const item: NavItem = { type: 'item', to: '/cluster', label: 'Cluster', feature: 'swarm_dashboard' }
    expect(isNavItemVisible(item, ctx({ hasFeature: () => false }))).toBe(false)
  })

  it('shows a runtime-scoped item when the runtime is available', () => {
    const item: NavItem = { type: 'item', to: '/containers', label: 'Containers', runtime: ['docker'] }
    expect(isNavItemVisible(item, ctx({ availableRuntimes: ['docker', 'swarm'] }))).toBe(true)
  })

  it('hides a runtime-scoped item when the runtime is not available', () => {
    const item: NavItem = { type: 'item', to: '/pods', label: 'Pods', runtime: ['kubernetes'] }
    expect(isNavItemVisible(item, ctx({ availableRuntimes: ['docker'] }))).toBe(false)
  })
})

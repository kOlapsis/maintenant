// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import type { Component } from 'vue'

export interface NavItem {
  type: string
  to?: string
  label?: string
  icon?: Component
  feature?: string
  runtime?: string[]
  hideInDemo?: boolean
}

export interface NavVisibilityContext {
  hasFeature: (feature: string) => boolean
  availableRuntimes: string[]
  isDemo: boolean
}

export function isNavItemVisible(item: NavItem, ctx: NavVisibilityContext): boolean {
  if (item.feature && !ctx.hasFeature(item.feature)) return false
  if (item.runtime && !item.runtime.some((rt) => ctx.availableRuntimes.includes(rt))) {
    return false
  }
  if (item.hideInDemo && ctx.isDemo) return false
  return true
}

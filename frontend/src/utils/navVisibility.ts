// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

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

// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. See COMMERCIAL-LICENSE.md.

import type { Component } from 'vue'
import type { Severity } from '@/composables/useSeverity'

// Normalised shape consumed by StatusGrid / StatusTile / SeverityRow. Callers
// (e.g. the dashboard) map their domain objects → GridItem; the grid stays
// decoupled from any store type.
export interface GridItem {
  id: string
  severity: Severity
  name: string
  /** Short mono sub-text (tile) / row metric, e.g. "1.2% cpu", "valid 42d". */
  meta?: string
  /** Row type badge, e.g. "Container". */
  kind?: string
  /** Row description line. */
  description?: string
}

export interface GridGroup {
  key: string
  label: string
  icon?: Component
  items: GridItem[]
  /** Fully-healthy groups collapse by default ("the wall of green"). */
  collapsedByDefault?: boolean
}

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

import type { ChipTone } from '@/components/ui/listFilters'
import type { Endpoint } from '@/services/endpointApi'

const statusTones: Record<Endpoint['status'], ChipTone> = {
  up: 'ok',
  down: 'down',
  degraded: 'warn',
  unknown: 'unknown',
}

/** Row gutter colour. A stale endpoint reports last-known state, not live state. */
export function endpointTone(ep: Endpoint): ChipTone {
  if (ep.stale) return 'unknown'
  return statusTones[ep.status] ?? 'neutral'
}

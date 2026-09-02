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

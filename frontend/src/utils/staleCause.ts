// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

export interface StaleCause {
  agent_offline?: boolean
  runtime_unreachable?: boolean
}

export function staleLabel(o: StaleCause): string {
  return o.runtime_unreachable ? 'Runtime unreachable' : 'Agent offline'
}

export function staleShort(o: StaleCause): string {
  return o.runtime_unreachable ? 'unreachable' : 'offline'
}

export function staleTitle(o: StaleCause, lastKnown: string): string {
  return `${staleLabel(o)} · last known: ${lastKnown}`
}

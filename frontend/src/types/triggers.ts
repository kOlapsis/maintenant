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

export interface AlertTrigger {
  id: string
  name: string
  filter_severities: string
  filter_sources: string
  filter_scopes: string
  filter_tags: string
  enabled: boolean
  channel_ids: string[]
  created_at: string
  updated_at: string
}

export interface TriggerRequest {
  name: string
  filter_severities: string
  filter_sources: string
  filter_scopes: string
  filter_tags: string
  enabled: boolean
  channel_ids: string[]
}

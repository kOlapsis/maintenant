// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

export interface AlertTrigger {
  id: string
  name: string
  filter_severities: string
  filter_sources: string
  filter_scopes: string
  filter_tags: string
  enabled: boolean
  notify_on_resolve: boolean
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
  notify_on_resolve: boolean
  channel_ids: string[]
}

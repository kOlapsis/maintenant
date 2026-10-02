// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB']

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes)) return '—'
  if (bytes === 0) return '0 B'
  const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(1024))
  const idx = Math.min(Math.max(i, 0), BYTE_UNITS.length - 1)
  return `${(bytes / Math.pow(1024, idx)).toFixed(idx > 0 ? 1 : 0)} ${BYTE_UNITS[idx]}`
}

export function formatMetricValue(metric: string, value: number): string {
  if (!Number.isFinite(value)) return '—'
  switch (metric) {
    case 'cpu':
      return `${value.toFixed(1)}%`
    case 'memory':
    case 'swap':
    case 'disk_space':
      return formatBytes(value)
    case 'network_io':
    case 'disk_io':
      return `${formatBytes(value)}/s`
    case 'load':
      return value.toFixed(2)
    default:
      return value.toFixed(1)
  }
}

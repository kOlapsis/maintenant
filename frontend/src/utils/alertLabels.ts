// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

const CERTIFICATE_LABELS: Record<string, string> = {
  ocsp_revoked: 'Certificate revoked (OCSP)',
  expired: 'Certificate expired',
  expiring: 'Certificate expiring soon',
  chain_invalid: 'Certificate chain invalid',
  hostname_mismatch: 'Hostname mismatch',
}

const HOST_LABELS: Record<string, string> = {
  os_eol: 'OS end of support',
}

export function humanizeAlertType(source: string, alertType: string): string {
  if (source === 'certificate') {
    const label = CERTIFICATE_LABELS[alertType]
    if (label) return label
  }
  if (source === 'host') {
    const label = HOST_LABELS[alertType]
    if (label) return label
  }
  return alertType
}

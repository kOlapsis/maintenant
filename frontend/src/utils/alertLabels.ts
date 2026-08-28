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

const CERTIFICATE_LABELS: Record<string, string> = {
  ocsp_revoked: 'Certificate revoked (OCSP)',
  expired: 'Certificate expired',
  expiring: 'Certificate expiring soon',
  chain_invalid: 'Certificate chain invalid',
  hostname_mismatch: 'Hostname mismatch',
}

export function humanizeAlertType(source: string, alertType: string): string {
  if (source === 'certificate') {
    const label = CERTIFICATE_LABELS[alertType]
    if (label) return label
  }
  return alertType
}

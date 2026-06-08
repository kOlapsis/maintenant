// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package resource

import "github.com/kolapsis/maintenant/internal/hoststat"

// HostStatReader samples host CPU and memory via /proc. The implementation
// lives in the dependency-free internal/hoststat package so the remote agent
// collector can reuse it without importing this package.
type HostStatReader = hoststat.Reader

// NewHostStatReader creates a host stat reader and takes an initial sample.
func NewHostStatReader() *HostStatReader { return hoststat.NewReader() }

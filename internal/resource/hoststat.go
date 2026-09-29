// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package resource

import "github.com/kolapsis/maintenant/internal/hoststat"

// HostStatReader samples host CPU and memory via /proc. The implementation
// lives in the dependency-free internal/hoststat package so the remote agent
// collector can reuse it without importing this package.
type HostStatReader = hoststat.Reader

// NewHostStatReader creates a host stat reader and takes an initial sample.
func NewHostStatReader() *HostStatReader { return hoststat.NewReader() }

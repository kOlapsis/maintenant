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

package extension

import "context"

// MultiHostAgent is the interface for the optional multi-host server/agent extension.
// CE uses NoopMultiHostAgent. Pro overrides via build injection.
type MultiHostAgent interface {
	StartServer(ctx context.Context) error
	StartAgent(ctx context.Context) error
	Stop() error
}

// NoopMultiHostAgent is the CE default — returns ErrNotAvailable for server/agent modes.
type NoopMultiHostAgent struct{}

func (NoopMultiHostAgent) StartServer(_ context.Context) error { return ErrNotAvailable }
func (NoopMultiHostAgent) StartAgent(_ context.Context) error  { return ErrNotAvailable }
func (NoopMultiHostAgent) Stop() error                         { return nil }

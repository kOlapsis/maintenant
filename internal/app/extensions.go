// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app

import "github.com/kolapsis/maintenant/internal/extpoint"

// Option configures New.
type Option func(*App)

// WithExtensions plugs licensed implementations into the application.
func WithExtensions(s extpoint.Set) Option {
	return func(a *App) { a.ext = s }
}

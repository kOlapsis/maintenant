// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import "time"

// SetSilenceRecheckInterval sets how often the engine looks for held alerts to promote. Call it before Start.
func (e *Engine) SetSilenceRecheckInterval(d time.Duration) { e.recheckEvery = d }

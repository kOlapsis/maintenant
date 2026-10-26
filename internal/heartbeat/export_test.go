// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package heartbeat

import "context"

// RunDeadlineCheck runs one pass of the deadline checker, as its ticker does.
func RunDeadlineCheck(ctx context.Context, s *Service) { s.checkDeadlines(ctx) }

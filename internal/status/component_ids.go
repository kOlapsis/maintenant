// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package status

import (
	"context"
	"fmt"
)

// InvalidComponentIDsError reports a component list naming an empty, unknown or repeated id.
type InvalidComponentIDsError struct {
	Reason string
}

func (e *InvalidComponentIDsError) Error() string { return e.Reason }

// CheckComponentIDs refuses, with an *InvalidComponentIDsError, a list whose ids do not each name a distinct existing component.
func CheckComponentIDs(ctx context.Context, components ComponentStore, ids []string) error {
	seen := make(map[string]bool, len(ids))
	for i, id := range ids {
		if id == "" {
			return &InvalidComponentIDsError{Reason: fmt.Sprintf("component_ids[%d] is empty", i)}
		}
		if seen[id] {
			return &InvalidComponentIDsError{Reason: fmt.Sprintf("component_ids[%d] repeats %q", i, id)}
		}
		seen[id] = true
		c, err := components.GetComponent(ctx, id)
		if err != nil {
			return fmt.Errorf("look up component %q: %w", id, err)
		}
		if c == nil {
			return &InvalidComponentIDsError{Reason: fmt.Sprintf("component_ids[%d]: no component %q", i, id)}
		}
	}
	return nil
}

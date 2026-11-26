// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package escalation

import (
	"context"
	"slices"

	"github.com/kolapsis/maintenant/internal/extension"
)

// PolicyLister lists the stored escalation policies.
type PolicyLister interface {
	ListPolicies(ctx context.Context, activeOnly bool) ([]*Policy, error)
}

// PoliciesUsingChannel returns the names of the policies, active or not, that notify channelID at one of their levels, in an edition that lets them be edited.
func PoliciesUsingChannel(ctx context.Context, policies PolicyLister, channelID string) ([]string, error) {
	if policies == nil || !extension.Allows(extension.CapAlertEscalation) {
		return nil, nil
	}
	all, err := policies.ListPolicies(ctx, false)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range all {
		if slices.ContainsFunc(p.Levels, func(l Level) bool { return slices.Contains(l.ChannelIDs, channelID) }) {
			names = append(names, p.Name)
		}
	}
	return names, nil
}

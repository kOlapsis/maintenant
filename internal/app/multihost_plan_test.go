package app

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/extension"
)

// The mode gate is the only refusal to start in the product. A Personal
// instance whose update window closed falls back to Community, and letting the
// gate see that would take a whole fleet's monitoring down over an unpaid
// renewal. What the degradation closes is real and lives elsewhere: every route
// behind requireCapability(CapMultihost) still refuses.
func TestMultihostPlanPermitted(t *testing.T) {
	cases := []struct {
		name    string
		granted bool
		status  string
		want    bool
	}{
		{"capability granted", true, "active", true},
		{"capability granted, window in grace", true, "update_window_grace", true},
		{"community, no license", false, "", false},
		{"community, license expired", false, "expired", false},
		{"community, license revoked", false, "revoked", false},
		{"community, server unreachable", false, "unreachable", false},
		{"bridled by a closed update window", false, extension.LicenseStatusUpdateWindowEnded, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, multihostPlanPermitted(c.granted, c.status))
		})
	}
}

// TestMultihostPlanPermitted_GraceIsNotADegradation: during the grace the
// edition is untouched, so the capability is still granted and the degraded
// branch must never be the reason the plan runs.
func TestMultihostPlanPermitted_GraceIsNotADegradation(t *testing.T) {
	assert.False(t, multihostPlanPermitted(false, "update_window_grace"),
		"a Community instance must not be let through by the grace status")
}

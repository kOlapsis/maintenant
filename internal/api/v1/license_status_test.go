package v1

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
	"github.com/kolapsis/maintenant/internal/license"
)

// GET /api/v1/license/status is what the banner and the Editions page read. Two
// things must hold whatever the state: the same keys are always present, and a
// date the license does not carry renders as an empty string, never as year 1.

func TestLicenseStatusPayload_KeysAreAlwaysPresent(t *testing.T) {
	want := []string{
		"status", "edition", "plan", "message",
		"verified_at", "expires_at", "updates_until", "update_grace_until",
	}

	states := map[string]*license.State{
		"no license configured": {Status: "inactive"},
		"active personal":       {Status: "active", Edition: extension.Personal, Plan: "personal"},
		"window grace":          {Status: license.StatusUpdateWindowGrace, Edition: extension.Personal},
		"window ended":          {Status: license.StatusUpdateWindowEnded, Edition: extension.Community},
	}

	for name, state := range states {
		t.Run(name, func(t *testing.T) {
			body := licenseStatusPayload(state, state.Edition)
			for _, key := range want {
				assert.Contains(t, body, key)
			}
			assert.Len(t, body, len(want), "an unexpected key would go undocumented")
		})
	}
}

// TestLicenseStatusPayload_AbsentDatesRenderEmpty: a Personal license is
// perpetual and an instance inside its window has no grace deadline. Neither
// may surface as 0001-01-01.
func TestLicenseStatusPayload_AbsentDatesRenderEmpty(t *testing.T) {
	body := licenseStatusPayload(&license.State{
		Status:  "active",
		Edition: extension.Personal,
		Plan:    "personal",
	}, extension.Personal)

	assert.Equal(t, "", body["expires_at"])
	assert.Equal(t, "", body["updates_until"])
	assert.Equal(t, "", body["update_grace_until"])
	assert.Equal(t, "", body["verified_at"])
}

func TestLicenseStatusPayload_WindowDatesAreExposed(t *testing.T) {
	windowEnd := time.Date(2027, time.August, 9, 12, 0, 0, 0, time.UTC)
	graceEnd := time.Date(2027, time.October, 1, 8, 0, 0, 0, time.UTC)

	body := licenseStatusPayload(&license.State{
		Status:           license.StatusUpdateWindowGrace,
		Edition:          extension.Personal,
		Plan:             "personal",
		Message:          "…Renew your Personal updates…",
		UpdatesUntil:     windowEnd,
		UpdateGraceUntil: graceEnd,
	}, extension.Personal)

	assert.Equal(t, license.StatusUpdateWindowGrace, body["status"])
	assert.Equal(t, "personal", body["edition"], "the grace keeps the edition")
	assert.Equal(t, "2027-08-09T12:00:00Z", body["updates_until"])
	assert.Equal(t, "2027-10-01T08:00:00Z", body["update_grace_until"])
}

// TestLicenseStatusPayload_EndedReportsCommunity: once the grace is spent the
// instance really is Community, and the route must say so rather than repeat
// what was bought.
func TestLicenseStatusPayload_EndedReportsCommunity(t *testing.T) {
	body := licenseStatusPayload(&license.State{
		Status:       license.StatusUpdateWindowEnded,
		Edition:      extension.Community,
		Plan:         "personal",
		UpdatesUntil: time.Date(2027, time.August, 9, 12, 0, 0, 0, time.UTC),
	}, extension.Community)

	require.Equal(t, license.StatusUpdateWindowEnded, body["status"])
	assert.Equal(t, "community", body["edition"])
	assert.Equal(t, "2027-08-09T12:00:00Z", body["updates_until"],
		"the date the user has to renew against stays visible")
}

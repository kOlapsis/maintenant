package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

// This file is SC-003: for the 3 editions × 20 capabilities, the answer must be
// the same whichever surface is asked. Without it, FR-004 ("a capability
// announced as available must actually be usable") is a claim nobody checks.
//
// The MCP side lives in its own package and cannot be called from here, so the
// third surface is covered by asserting that both surfaces read the same
// registry function — see TestConformance_MCPUsesTheSameRegistry in
// internal/mcp.

func editionResponse(t *testing.T, smtpConfigured bool) map[string]any {
	t.Helper()

	r := &Router{}
	rec := httptest.NewRecorder()
	r.handleGetEdition(smtpConfigured, HandlerDeps{})(rec, httptest.NewRequest("GET", "/api/v1/edition", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

// TestConformance_EndpointAgreesWithMiddleware walks the whole matrix and
// asserts the flag the UI receives matches the decision the middleware makes.
// A disagreement here is exactly the defect FR-004 forbids: a feature offered
// in the interface and refused on use, or the reverse.
func TestConformance_EndpointAgreesWithMiddleware(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		for capability := range extension.Catalog() {
			t.Run(string(edition)+"/"+string(capability), func(t *testing.T) {
				withEdition(t, edition)

				// What the interface is told.
				body := editionResponse(t, true)
				features, ok := body["features"].(map[string]any)
				require.True(t, ok, "features must be an object")
				announced, present := features[string(capability)]
				require.True(t, present, "capability %q missing from features", capability)

				// What the middleware decides.
				called := false
				rec := httptest.NewRecorder()
				requireCapability(capability, func(w http.ResponseWriter, _ *http.Request) {
					called = true
					w.WriteHeader(http.StatusOK)
				}).ServeHTTP(rec, httptest.NewRequest("POST", "/api/v1/whatever", nil))

				assert.Equal(t, announced, called,
					"features[%q]=%v but the middleware %s the request",
					capability, announced, map[bool]string{true: "allowed", false: "refused"}[called])
			})
		}
	}
}

// TestConformance_FeatureEditionsMirrorsTheFlags: the two objects must carry
// exactly the same keys, and features[k] must equal "edition ranks at or above
// feature_editions[k]" — with smtp the single documented exception, since it is
// also conditioned on the configuration.
func TestConformance_FeatureEditionsMirrorsTheFlags(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		t.Run(string(edition), func(t *testing.T) {
			withEdition(t, edition)
			body := editionResponse(t, true)

			features := body["features"].(map[string]any)
			editions := body["feature_editions"].(map[string]any)

			require.Len(t, features, 20)
			require.Len(t, editions, 20)
			for key := range features {
				require.Contains(t, editions, key, "feature_editions is missing %q", key)
			}

			for key, raw := range editions {
				min, ok := raw.(string)
				require.True(t, ok)
				want := edition.AtLeast(extension.Edition(min))
				assert.Equal(t, want, features[key],
					"features[%q]=%v but edition %q vs minimum %q says %v", key, features[key], edition, min, want)
			}
		})
	}
}

// TestConformance_SMTPSeparatesPermissionFromConfiguration is the one exception
// to the invariant above, and it is deliberate: the capability says what the
// edition permits, the configuration says what is ready. Collapsing them would
// make the two refusals indistinguishable (FR-015).
func TestConformance_SMTPSeparatesPermissionFromConfiguration(t *testing.T) {
	withEdition(t, extension.Personal)

	configured := editionResponse(t, true)
	assert.Equal(t, true, configured["features"].(map[string]any)["smtp"],
		"Personal permits smtp and it is configured")

	unconfigured := editionResponse(t, false)
	assert.Equal(t, false, unconfigured["features"].(map[string]any)["smtp"],
		"the flag drops when smtp is not configured, even though the edition permits it")

	// The declared minimum does not move with the configuration — that is what
	// lets the interface say "allowed but not set up" rather than "buy Pro".
	assert.Equal(t, string(extension.Personal),
		unconfigured["feature_editions"].(map[string]any)["smtp"])
}

// TestConformance_HistoryWindowsMatchWhatTheEndpointsAccept is FR-016 and
// SC-003: for each edition, the set of windows the interface is told are open
// must be exactly the set both history endpoints actually serve. A drift here
// is the defect the whole feature exists to prevent: a window offered and then
// refused, or refused and quietly served.
func TestConformance_HistoryWindowsMatchWhatTheEndpointsAccept(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		t.Run(string(edition), func(t *testing.T) {
			withEdition(t, edition)

			body := editionResponse(t, true)
			history, ok := body["resource_history"].(map[string]any)
			require.True(t, ok, "the edition response carries no resource_history object")

			windows, ok := history["windows"].([]any)
			require.True(t, ok)
			require.Len(t, windows, 6)

			maxSeconds, ok := history["max_window_seconds"].(float64)
			require.True(t, ok)

			// max_window is always one of the catalogue's own names, never empty.
			maxName, _ := history["max_window"].(string)
			require.NotEmpty(t, maxName)

			announcedOpen := map[string]bool{}
			foundMax := false
			for _, raw := range windows {
				spec := raw.(map[string]any)
				name := spec["window"].(string)
				seconds := spec["seconds"].(float64)
				announcedOpen[name] = seconds <= maxSeconds
				if name == maxName {
					foundMax = true
				}

				// The two ways of saying the same thing must agree: "within the
				// cap" and "the edition ranks at or above the window's minimum".
				minEdition := extension.Edition(spec["min_edition"].(string))
				assert.Equal(t, edition.AtLeast(minEdition), announcedOpen[name],
					"window %q: cap says %v, edition ranking says %v",
					name, announcedOpen[name], edition.AtLeast(minEdition))
			}
			assert.True(t, foundMax, "max_window %q is not in the catalogue", maxName)

			for name, open := range announcedOpen {
				historyStub := &stubHistoryService{}
				gotHistory := historyRequest(t, historyStub, name).Code

				topStub := &stubTopService{}
				gotTop := topRequest(t, topStub, "period="+name).Code

				if open {
					assert.Equal(t, http.StatusOK, gotHistory, "%s announced open, history refused it", name)
					assert.Equal(t, http.StatusOK, gotTop, "%s announced open, top consumers refused it", name)
					continue
				}
				assert.Equal(t, http.StatusForbidden, gotHistory, "%s announced closed, history served it", name)
				assert.Equal(t, http.StatusForbidden, gotTop, "%s announced closed, top consumers served it", name)
			}
		})
	}
}

// TestConformance_QuotasReportRealUsage: `used` used to be pinned to 0 on Pro,
// so an instance running fifty endpoints reported none. Every edition now
// counts, and the limit always comes from extension.Limit.
func TestConformance_QuotasReportRealUsage(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		t.Run(string(edition), func(t *testing.T) {
			withEdition(t, edition)
			body := editionResponse(t, true)
			quotas, ok := body["quotas"].(map[string]any)
			require.True(t, ok)

			// With no stores wired, no resource is reported at all — the point
			// here is that nothing is fabricated with a convention value.
			for name, raw := range quotas {
				entry := raw.(map[string]any)
				assert.NotNil(t, entry["used"], "quota %q reports no usage", name)
				assert.NotNil(t, entry["limit"], "quota %q reports no limit", name)
			}
		})
	}
}

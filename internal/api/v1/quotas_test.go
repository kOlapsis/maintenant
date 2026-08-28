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

// decodeRefusal reads the structured error body from a refusal.
func decodeRefusal(t *testing.T, rec *httptest.ResponseRecorder) ErrorDetail {
	t.Helper()
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body.Error
}

// TestRefuseQuota_CarriesEverythingTheUINeeds: the four capped resources answer
// with the same shape, and the fields come from extension.Limit rather than
// from a literal in the message. This is what lets the interface stop matching
// on "Upgrade to Pro" (FR-019, FR-020).
func TestRefuseQuota_CarriesEverythingTheUINeeds(t *testing.T) {
	withEdition(t, extension.Community)

	cases := []struct {
		resource  extension.Resource
		wantLimit int
	}{
		{extension.ResourceEndpoints, 10},
		{extension.ResourceHeartbeats, 5},
		{extension.ResourceCertificates, 5},
		{extension.ResourceStatusComponents, 3},
	}

	for _, c := range cases {
		t.Run(string(c.resource), func(t *testing.T) {
			rec := httptest.NewRecorder()
			refuseQuota(rec, c.resource)

			assert.Equal(t, http.StatusForbidden, rec.Code)
			detail := decodeRefusal(t, rec)
			assert.Equal(t, "QUOTA_EXCEEDED", detail.Code)
			assert.Equal(t, string(c.resource), detail.Resource)
			require.NotNil(t, detail.Limit)
			assert.Equal(t, c.wantLimit, *detail.Limit)
			assert.Equal(t, string(extension.Personal), detail.RequiredEdition,
				"Personal lifts every resource cap except agent hosts")
			assert.NotContains(t, detail.Message, "Upgrade to Pro")
		})
	}
}

// TestRefuseQuota_AgentHostsNameTheRightEdition: agent hosts are the one
// resource Personal does not make unlimited, so the edition that lifts the cap
// depends on where you already are.
func TestRefuseQuota_AgentHostsNameTheRightEdition(t *testing.T) {
	t.Run("personal at its cap is told Pro", func(t *testing.T) {
		withEdition(t, extension.Personal)
		rec := httptest.NewRecorder()
		writeQuotaRefusal(rec, http.StatusConflict, "HOST_LIMIT_REACHED", extension.ResourceAgentHosts)

		assert.Equal(t, http.StatusConflict, rec.Code)
		detail := decodeRefusal(t, rec)
		assert.Equal(t, "HOST_LIMIT_REACHED", detail.Code)
		assert.Equal(t, "agent_hosts", detail.Resource)
		require.NotNil(t, detail.Limit)
		assert.Equal(t, 20, *detail.Limit)
		assert.Equal(t, string(extension.Pro), detail.RequiredEdition)
	})

	t.Run("community is told Personal", func(t *testing.T) {
		withEdition(t, extension.Community)
		rec := httptest.NewRecorder()
		writeQuotaRefusal(rec, http.StatusConflict, "HOST_LIMIT_REACHED", extension.ResourceAgentHosts)

		detail := decodeRefusal(t, rec)
		require.NotNil(t, detail.Limit)
		assert.Equal(t, 0, *detail.Limit)
		assert.Equal(t, string(extension.Personal), detail.RequiredEdition)
	})
}

// TestComputeQuotas_LimitsComeFromTheRegistry walks the three editions and
// asserts the reported limit is exactly extension.Limit. The values used to be
// literals here, able to drift from the ones that actually refuse a creation.
func TestComputeQuotas_LimitsComeFromTheRegistry(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		t.Run(string(edition), func(t *testing.T) {
			withEdition(t, edition)

			r := &Router{}
			rec := httptest.NewRecorder()
			r.handleGetEdition(true, HandlerDeps{})(rec, httptest.NewRequest("GET", "/api/v1/edition", nil))

			var body struct {
				Quotas map[string]struct {
					Used  int `json:"used"`
					Limit int `json:"limit"`
				} `json:"quotas"`
			}
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))

			for name, entry := range body.Quotas {
				assert.Equal(t, extension.Limit(extension.Resource(name)), entry.Limit,
					"quota %q reports a limit that is not the declared one", name)
			}
		})
	}
}

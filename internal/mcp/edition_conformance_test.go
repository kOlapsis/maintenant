package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/extension"
)

// The MCP half of SC-003. The REST half lives in internal/api/v1; together they
// cover the three surfaces named by FR-004.
//
// Before this feature the MCP tools carried their own vocabulary
// (status_page_incidents, cve_intelligence, kubernetes_nodes…), so "the same
// capability" could not even be stated across surfaces, let alone compared.

// TestConformance_CheckCapabilityMatchesTheRegistry walks the whole matrix and
// asserts the MCP gate decides exactly what the registry declares — the same
// table the REST middleware reads.
func TestConformance_CheckCapabilityMatchesTheRegistry(t *testing.T) {
	for _, edition := range []extension.Edition{extension.Community, extension.Personal, extension.Pro} {
		for capability, min := range extension.Catalog() {
			t.Run(string(edition)+"/"+string(capability), func(t *testing.T) {
				withEdition(t, edition)

				result, _, err := checkCapability(capability)
				require.NoError(t, err)

				if edition.AtLeast(min) {
					assert.Nil(t, result, "capability %q must be open on %q", capability, edition)
					return
				}

				require.NotNil(t, result, "capability %q must be refused on %q", capability, edition)
				assert.True(t, result.IsError)

				var payload struct {
					Error           string `json:"error"`
					Feature         string `json:"feature"`
					RequiredEdition string `json:"required_edition"`
					Message         string `json:"message"`
				}
				require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &payload))

				assert.Equal(t, "edition_required", payload.Error)
				assert.Equal(t, string(capability), payload.Feature,
					"the refusal must name the capability using the REST vocabulary")
				assert.Equal(t, string(min), payload.RequiredEdition,
					"the refusal must name the edition that grants it, not Pro by default")
				assert.NotContains(t, payload.Message, "maintenant.dev",
					"the refusal names the edition required; it does not advertise")
			})
		}
	}
}

// TestConformance_VocabularyIsTheRESTVocabulary pins the realignment. These
// names are what makes the two surfaces comparable at all, so a drift here is
// a contract break, not a cosmetic change.
func TestConformance_VocabularyIsTheRESTVocabulary(t *testing.T) {
	for _, name := range []string{
		"incidents",           // was status_page_incidents
		"maintenance_windows", // was status_page_maintenance
		"cve_enrichment",      // was cve_intelligence
		"k8s_cluster",         // was kubernetes_nodes
		"swarm_dashboard",     // was swarm_nodes
		"risk_scoring",
		"security_posture",
		"alert_escalation",
		"alert_advanced_filters",
	} {
		_, declared := extension.Catalog()[extension.Capability(name)]
		assert.True(t, declared, "capability %q is not in the registry", name)
	}
}

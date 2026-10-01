package mcp

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
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

// A tool whose description says it requires an edition refuses below it.
func TestConformance_EveryGatedToolRefuses(t *testing.T) {
	withEdition(t, extension.Community)
	ctx := context.Background()

	server := newTestServer(t)
	ct, st := gomcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := gomcp.NewClient(&gomcp.Implementation{Name: "conformance", Version: "0"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })

	tools, err := cs.ListTools(ctx, nil)
	require.NoError(t, err)

	gated := 0
	for _, tool := range tools.Tools {
		m := requiresEdition.FindStringSubmatch(tool.Description)
		if m == nil {
			continue
		}
		required := extension.Edition(strings.ToLower(m[1]))
		if !required.AtLeast(extension.Personal) {
			continue
		}
		gated++
		t.Run(tool.Name, func(t *testing.T) {
			result, err := cs.CallTool(ctx, &gomcp.CallToolParams{Name: tool.Name, Arguments: placeholderArgs(t, tool.InputSchema)})
			require.NoError(t, err)
			require.True(t, result.IsError, "%s answered on Community", tool.Name)

			var payload struct {
				Error           string `json:"error"`
				Feature         string `json:"feature"`
				RequiredEdition string `json:"required_edition"`
			}
			require.NoError(t, json.Unmarshal([]byte(textFromContent(t, result.Content)), &payload))
			assert.Equal(t, "edition_required", payload.Error)
			assert.Equal(t, string(required), payload.RequiredEdition)
			_, declared := extension.Catalog()[extension.Capability(payload.Feature)]
			assert.True(t, declared, "feature %q is not a registered capability", payload.Feature)
		})
	}
	assert.NotZero(t, gated, "no gated tool found: the description convention changed")
}

var requiresEdition = regexp.MustCompile(`Requires the (\w+) edition\.$`)

// placeholderArgs fills each required property so the call gets past input validation.
func placeholderArgs(t *testing.T, schema any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(schema)
	require.NoError(t, err)
	var s struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(raw, &s))

	args := map[string]any{}
	for _, name := range s.Required {
		types := []string{}
		switch v := s.Properties[name].Type.(type) {
		case string:
			types = append(types, v)
		case []any:
			for _, x := range v {
				if str, ok := x.(string); ok && str != "null" {
					types = append(types, str)
				}
			}
		}
		require.NotEmpty(t, types, "property %s has no type", name)
		switch types[0] {
		case "string":
			args[name] = "x"
		case "integer", "number":
			args[name] = 1
		case "boolean":
			args[name] = false
		case "array":
			args[name] = []any{}
		case "object":
			args[name] = map[string]any{}
		default:
			t.Fatalf("property %s has unexpected type %q", name, types[0])
		}
	}
	return args
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

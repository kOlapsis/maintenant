package webhook

import (
	"testing"

	"github.com/kolapsis/maintenant/internal/event"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// mapSSETypeToWebhookEvent
// ---------------------------------------------------------------------------

func TestMapSSETypeToWebhookEvent_ContainerEvents(t *testing.T) {
	// All container-related SSE events map to a single webhook event type.
	inputs := []string{
		event.ContainerStateChanged,
		event.ContainerDiscovered,
		"container.removed",
	}
	for _, input := range inputs {
		result := mapSSETypeToWebhookEvent(input)
		assert.Equal(t, event.ContainerStateChanged, result, "input: %s", input)
	}
}

func TestMapSSETypeToWebhookEvent_EndpointEvents(t *testing.T) {
	inputs := []string{
		event.EndpointStatusChanged,
		event.EndpointDiscovered,
		event.EndpointRemoved,
	}
	for _, input := range inputs {
		result := mapSSETypeToWebhookEvent(input)
		assert.Equal(t, event.EndpointStatusChanged, result, "input: %s", input)
	}
}

func TestMapSSETypeToWebhookEvent_DirectMappings(t *testing.T) {
	cases := map[string]string{
		event.HeartbeatStatusChanged:   event.HeartbeatStatusChanged,
		event.CertificateStatusChanged: event.CertificateStatusChanged,
		event.AlertFired:               event.AlertFired,
		event.AlertResolved:            event.AlertResolved,
	}
	for input, expected := range cases {
		assert.Equal(t, expected, mapSSETypeToWebhookEvent(input), "input: %s", input)
	}
}

func TestMapSSETypeToWebhookEvent_UnknownReturnsEmpty(t *testing.T) {
	unknowns := []string{
		"resource.snapshot",
		"security.insights_changed",
		"",
		"totally.unknown",
	}
	for _, input := range unknowns {
		assert.Empty(t, mapSSETypeToWebhookEvent(input), "unknown SSE type %q should map to empty", input)
	}
}

// ---------------------------------------------------------------------------
// matchesEventTypes
// ---------------------------------------------------------------------------

func TestMatchesEventTypes_WildcardMatchesEverything(t *testing.T) {
	subscribed := []string{"*"}

	assert.True(t, matchesEventTypes(subscribed, event.ContainerStateChanged))
	assert.True(t, matchesEventTypes(subscribed, event.AlertFired))
	assert.True(t, matchesEventTypes(subscribed, "anything.at.all"))
}

func TestMatchesEventTypes_ExactMatch(t *testing.T) {
	subscribed := []string{event.AlertFired, event.AlertResolved}

	assert.True(t, matchesEventTypes(subscribed, event.AlertFired))
	assert.True(t, matchesEventTypes(subscribed, event.AlertResolved))
	assert.False(t, matchesEventTypes(subscribed, event.ContainerStateChanged))
}

func TestMatchesEventTypes_EmptySubscribedMatchesNothing(t *testing.T) {
	assert.False(t, matchesEventTypes(nil, event.AlertFired))
	assert.False(t, matchesEventTypes([]string{}, event.AlertFired))
}

func TestMatchesEventTypes_WildcardAmongSpecific(t *testing.T) {
	// Wildcard in the list should still match everything.
	subscribed := []string{event.AlertFired, "*"}

	assert.True(t, matchesEventTypes(subscribed, "anything"))
}

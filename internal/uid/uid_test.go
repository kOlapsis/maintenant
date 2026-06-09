// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package uid

import (
	"testing"

	"github.com/google/uuid"
)

func TestNew_IsUUIDv7AndUnique(t *testing.T) {
	a, b := New(), New()
	if a == b {
		t.Fatalf("New() returned a duplicate: %s", a)
	}
	ua, err := uuid.Parse(a)
	if err != nil {
		t.Fatalf("New() is not a uuid: %v", err)
	}
	if ua.Version() != 7 {
		t.Fatalf("New() version = %d, want 7", ua.Version())
	}
}

func TestDerive_Deterministic(t *testing.T) {
	x := Derive(nsContainer, "agent-1", "abc")
	y := Derive(nsContainer, "agent-1", "abc")
	if x != y {
		t.Fatalf("Derive not deterministic: %s vs %s", x, y)
	}
	u, err := uuid.Parse(x)
	if err != nil {
		t.Fatalf("Derive is not a uuid: %v", err)
	}
	if u.Version() != 5 {
		t.Fatalf("Derive version = %d, want 5", u.Version())
	}
}

func TestDerive_DistinctInputs(t *testing.T) {
	ids := []string{
		Derive(nsContainer, "agent-1", "abc"),
		Derive(nsContainer, "agent-2", "abc"),     // different agent
		Derive(nsContainer, "agent-1", "abcd"),    // different key
		Derive(nsContainer, "agent-1", "ab", "c"), // different boundary vs "abc"
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if seen[id] {
			t.Fatalf("collision at index %d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestDerive_NamespaceMatters(t *testing.T) {
	if Derive(nsContainer, "a") == Derive(nsEndpoint, "a") {
		t.Fatal("different namespaces produced the same id")
	}
}

func TestDerive_TrimsParts(t *testing.T) {
	if Derive(nsCert, "agent", "host") != Derive(nsCert, " agent ", "  host  ") {
		t.Fatal("Derive should trim whitespace in parts")
	}
}

func TestHelpers_MatchDerive(t *testing.T) {
	if Container("a", "x") != Derive(nsContainer, "a", "x") {
		t.Fatal("Container mismatch")
	}
	if EndpointLabel("a", "c", "k") != Derive(nsEndpoint, "a", "c", "k") {
		t.Fatal("EndpointLabel mismatch")
	}
	if CertMonitor("a", "h", 443) != Derive(nsCert, "a", "h", "443") {
		t.Fatal("CertMonitor mismatch")
	}
	if SwarmNode("a", "n") != Derive(nsSwarmNode, "a", "n") {
		t.Fatal("SwarmNode mismatch")
	}
}

func TestAgent_DefaultsToLocal(t *testing.T) {
	if Agent("") != LocalAgent {
		t.Fatal("empty agent should map to LocalAgent")
	}
	if Agent("  ") != LocalAgent {
		t.Fatal("blank agent should map to LocalAgent")
	}
	if Agent("agent-9") != "agent-9" {
		t.Fatal("non-empty agent should pass through unchanged")
	}
}

func TestLocalAgent_IsNilUUID(t *testing.T) {
	if LocalAgent != uuid.Nil.String() {
		t.Fatalf("LocalAgent = %s, want %s", LocalAgent, uuid.Nil.String())
	}
}

// TestGoldenVectors pins namespaces and derived ids so an accidental change to
// nsRoot or any namespace is caught. These values must never change — doing so
// would orphan every previously persisted id.
func TestGoldenVectors(t *testing.T) {
	cases := map[string]struct{ got, want string }{
		"nsContainer":             {nsContainer.String(), "41ca99dd-4758-5d5b-abd0-95d3001c420b"},
		"nsEndpoint":              {nsEndpoint.String(), "00687627-cd80-5860-9daa-f9db4701dbb1"},
		"nsCert":                  {nsCert.String(), "31db2c95-c368-59df-b018-e6ecf09bbfd3"},
		"nsSwarmNode":             {nsSwarmNode.String(), "80f18242-7658-5d48-9e4a-c6f1a2d94d3f"},
		"container(local,abc123)": {Container(LocalAgent, "abc123"), "51091fab-cae2-5ca2-a58d-7b2459a1afa8"},
		"endpoint(a,web,health)":  {EndpointLabel("a", "web", "maintenant.endpoint"), "70d87cef-16b7-5ecc-8914-7e4c1fe10d02"},
		"cert(a,example.com,443)": {CertMonitor("a", "example.com", 443), "be26f414-880f-5477-90dd-bf161c76f60e"},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %s, want %s (namespace/derivation drift!)", name, c.got, c.want)
		}
	}
}

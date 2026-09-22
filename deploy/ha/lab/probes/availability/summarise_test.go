package main

import "testing"

func TestSummarise(t *testing.T) {
	tests := []struct {
		name          string
		samples       []sample
		wantRTOMs     int64
		wantRecovered bool
		wantOutage    bool
	}{
		{
			name: "no outage",
			samples: []sample{
				{Wall: "2027-01-15T10:00:00Z", Epoch: "e1", MonoNS: 0, Outcome: outcomeUp},
				{Wall: "2027-01-15T10:00:01Z", Epoch: "e1", MonoNS: 1_000_000_000, Outcome: outcomeUp},
				{Wall: "2027-01-15T10:00:02Z", Epoch: "e1", MonoNS: 2_000_000_000, Outcome: outcomeUp},
			},
			wantRTOMs:     0,
			wantRecovered: true,
			wantOutage:    false,
		},
		{
			name: "one outage",
			samples: []sample{
				{Wall: "2027-01-15T10:00:00Z", Epoch: "e1", MonoNS: 0, Outcome: outcomeUp},
				{Wall: "2027-01-15T10:00:01Z", Epoch: "e1", MonoNS: 1_000_000_000, Outcome: outcomeTimeout},
				{Wall: "2027-01-15T10:00:02Z", Epoch: "e1", MonoNS: 2_000_000_000, Outcome: outcomeTimeout},
				{Wall: "2027-01-15T10:00:03Z", Epoch: "e1", MonoNS: 3_000_000_000, Outcome: outcomeUp},
			},
			wantRTOMs:     2000,
			wantRecovered: true,
			wantOutage:    true,
		},
		{
			name: "two outages, longest wins",
			samples: []sample{
				{Wall: "2027-01-15T10:00:00Z", Epoch: "e1", MonoNS: 0, Outcome: outcomeUp},
				{Wall: "2027-01-15T10:00:01Z", Epoch: "e1", MonoNS: 1_000_000_000, Outcome: outcomeTimeout},
				{Wall: "2027-01-15T10:00:02Z", Epoch: "e1", MonoNS: 2_000_000_000, Outcome: outcomeUp},
				{Wall: "2027-01-15T10:00:03Z", Epoch: "e1", MonoNS: 3_000_000_000, Outcome: outcomeRefused},
				{Wall: "2027-01-15T10:00:04Z", Epoch: "e1", MonoNS: 4_000_000_000, Outcome: outcomeRefused},
				{Wall: "2027-01-15T10:00:05Z", Epoch: "e1", MonoNS: 5_000_000_000, Outcome: outcomeRefused},
				{Wall: "2027-01-15T10:00:06Z", Epoch: "e1", MonoNS: 6_000_000_000, Outcome: outcomeUp},
			},
			wantRTOMs:     3000,
			wantRecovered: true,
			wantOutage:    true,
		},
		{
			name: "outage across an epoch change",
			samples: []sample{
				{Wall: "2027-01-15T10:00:00Z", Epoch: "e1", MonoNS: 0, Outcome: outcomeUp},
				{Wall: "2027-01-15T10:00:01Z", Epoch: "e1", MonoNS: 1_000_000_000, Outcome: outcomeError},
				{Wall: "2027-01-15T10:00:02Z", Epoch: "e2", MonoNS: 200_000_000, Outcome: outcomeError},
				{Wall: "2027-01-15T10:00:03Z", Epoch: "e2", MonoNS: 1_200_000_000, Outcome: outcomeUp},
			},
			wantRTOMs:     2000,
			wantRecovered: true,
			wantOutage:    true,
		},
		{
			name: "window ends while down",
			samples: []sample{
				{Wall: "2027-01-15T10:00:00Z", Epoch: "e1", MonoNS: 0, Outcome: outcomeUp},
				{Wall: "2027-01-15T10:00:01Z", Epoch: "e1", MonoNS: 1_000_000_000, Outcome: outcomeTimeout},
				{Wall: "2027-01-15T10:00:02Z", Epoch: "e1", MonoNS: 2_000_000_000, Outcome: outcomeTimeout},
			},
			wantRTOMs:     1000,
			wantRecovered: false,
			wantOutage:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarise(tt.samples, "2027-01-15T10:00:00Z", "2027-01-15T10:01:00Z")
			if got.RTOMs != tt.wantRTOMs {
				t.Errorf("rto_ms = %d, want %d", got.RTOMs, tt.wantRTOMs)
			}
			if got.Recovered != tt.wantRecovered {
				t.Errorf("recovered = %v, want %v", got.Recovered, tt.wantRecovered)
			}
			hasOutage := got.OutageStart != "" || got.OutageEnd != ""
			if hasOutage != tt.wantOutage {
				t.Errorf("outage set = %v, want %v (start=%q end=%q)", hasOutage, tt.wantOutage, got.OutageStart, got.OutageEnd)
			}
			if got.Samples != len(tt.samples) {
				t.Errorf("samples = %d, want %d", got.Samples, len(tt.samples))
			}
		})
	}
}

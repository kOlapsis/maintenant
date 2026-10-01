// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import (
	"strings"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

// UpdateLabelPrefix starts every label or annotation that configures update tracking.
const UpdateLabelPrefix = "maintenant.update."

// UpdateLabels returns the update tracking labels among labels, nil when there are none.
func UpdateLabels(labels map[string]string) map[string]string {
	var out map[string]string
	for k, v := range labels {
		if !strings.HasPrefix(k, UpdateLabelPrefix) {
			continue
		}
		if out == nil {
			out = make(map[string]string)
		}
		out[k] = v
	}
	return out
}

// AgentDetails is what a remote agent last reported about one of its containers and the store does not keep.
type AgentDetails struct {
	UpdateLabels map[string]string
	RepoDigests  []string // "repo@sha256:..." of the image the container runs
	DigestsKnown bool     // the agent reported RepoDigests; known and empty marks an image never pulled from a registry
}

type agentContainerKey struct {
	agentID    string
	externalID string
}

// AgentDetails returns what agentID last reported about its container externalID, false until it has reported it since startup.
func (s *Service) AgentDetails(agentID, externalID string) (AgentDetails, bool) {
	s.agentDetailsMu.RLock()
	defer s.agentDetailsMu.RUnlock()
	d, ok := s.agentDetails[agentContainerKey{agentID, externalID}]
	return d, ok
}

func (s *Service) recordAgentDetails(agentID string, ev *agentpb.ContainerEvent) {
	key := agentContainerKey{agentID, ev.GetContainerId()}
	s.agentDetailsMu.Lock()
	defer s.agentDetailsMu.Unlock()
	d := s.agentDetails[key]
	if labels := ev.GetLabels(); len(labels) > 0 {
		d.UpdateLabels = UpdateLabels(labels)
	}
	if rd := ev.GetRepoDigests(); rd != nil {
		d.RepoDigests = rd.GetDigests()
		d.DigestsKnown = true
	}
	s.agentDetails[key] = d
}

func (s *Service) forgetAgentDetails(agentID, externalID string) {
	s.agentDetailsMu.Lock()
	defer s.agentDetailsMu.Unlock()
	delete(s.agentDetails, agentContainerKey{agentID, externalID})
}

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

package agent

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/kolapsis/maintenant/internal/agentpb"
)

func runtimeToProto(label string) agentpb.Runtime {
	switch label {
	case RuntimeDocker:
		return agentpb.Runtime_RUNTIME_DOCKER
	case RuntimeSwarm:
		return agentpb.Runtime_RUNTIME_SWARM
	case RuntimeKubernetes:
		return agentpb.Runtime_RUNTIME_KUBERNETES
	default:
		return agentpb.Runtime_RUNTIME_UNSPECIFIED
	}
}

// RunEnrollment sends a RegisterRequest to the server using the enrollment token.
// On success it marks the identity as registered and persists it to dataDir.
func RunEnrollment(
	ctx context.Context,
	id *Identity,
	dataDir string,
	enrollmentToken string,
	runtimeLabel string,
	label string,
	agentVersion string,
	client *Client,
) error {
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("get hostname: %w", err)
	}

	req := &agentpb.RegisterRequest{
		AgentId:         id.AgentID,
		PublicKey:       id.PublicKey,
		EnrollmentToken: enrollmentToken,
		Hostname:        hostname,
		Label:           label,
		OsArch:          runtime.GOOS + "/" + runtime.GOARCH,
		AgentVersion:    agentVersion,
		DetectedRuntime: runtimeToProto(runtimeLabel),
	}

	if _, err := client.Register(ctx, req); err != nil {
		return fmt.Errorf("enrollment failed: %w", err)
	}

	now := time.Now().UTC()
	id.Registered = true
	id.RegisteredAt = &now

	if err := id.Save(dataDir); err != nil {
		return fmt.Errorf("persist identity after enrollment: %w", err)
	}

	return nil
}

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

package agentserver

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
)

var (
	ErrBadSignature = errors.New("invalid ed25519 signature")
	ErrClockSkew    = errors.New("client clock skew exceeds 300s")
	ErrAgentRevoked = errors.New("agent is revoked")
	ErrAgentUnknown = errors.New("agent not found")
)

const maxClockSkewSeconds = 300

// GenerateChallenge creates a 32-byte random nonce for the auth handshake.
func GenerateChallenge() (*agentpb.AuthChallenge, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return &agentpb.AuthChallenge{Nonce: nonce}, nil
}

// Verify checks the AuthResponse signature and returns nil on success.
// Payload (byte-exact): nonce(32) || agent_uuid_bytes(16) || timestamp_be64(8).
func Verify(req *agentpb.AuthResponse, nonce []byte, ag *agent.Agent, serverNow time.Time) error {
	if ag == nil {
		return ErrAgentUnknown
	}
	if ag.Status == "revoked" {
		return ErrAgentRevoked
	}

	// Clock skew check.
	clientUnix := req.GetTimestamp()
	diff := serverNow.Unix() - clientUnix
	if diff < 0 {
		diff = -diff
	}
	if diff > maxClockSkewSeconds {
		return ErrClockSkew
	}

	// Build signing payload: nonce(32) || uuid_bytes(16) || timestamp_be64(8).
	uuidBytes, err := uuidToBytes(req.GetAgentId())
	if err != nil {
		return ErrBadSignature
	}

	payload := make([]byte, 56)
	copy(payload[0:32], nonce)
	copy(payload[32:48], uuidBytes)
	binary.BigEndian.PutUint64(payload[48:56], uint64(clientUnix))

	if !ed25519.Verify(ed25519.PublicKey(ag.PublicKey), payload, req.GetSignature()) {
		return ErrBadSignature
	}
	return nil
}

// uuidToBytes decodes a UUID-like hex string (with dashes) to 16 raw bytes.
func uuidToBytes(uuid string) ([]byte, error) {
	clean := strings.ReplaceAll(uuid, "-", "")
	if len(clean) != 32 {
		return nil, errors.New("invalid uuid length")
	}
	return hex.DecodeString(clean)
}

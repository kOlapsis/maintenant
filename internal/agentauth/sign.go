// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

// Package agentauth defines the byte-exact wire format used by the gRPC agent
// authentication handshake. Both the agent (signer) and the agent server
// (verifier) depend on this package to guarantee they agree on the payload
// structure — touch one side without the other and the handshake stops working.
package agentauth

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strings"
)

// SignPayloadSize is the fixed length of the agent auth signing payload.
// nonce(32) || agent_uuid_bytes(16) || timestamp_be64(8) = 56 bytes.
const SignPayloadSize = 56

// ErrInvalidUUID is returned when a UUID-like string cannot be decoded to 16 bytes.
var ErrInvalidUUID = errors.New("invalid uuid: expected 32 hex chars after stripping dashes")

// BuildSignPayload constructs the byte-exact payload that the agent signs and
// the server verifies during the Push stream auth handshake.
// Layout: nonce(32) || uuid_bytes(16) || timestamp_be64(8).
func BuildSignPayload(nonce []byte, agentID string, ts int64) ([]byte, error) {
	uuidBytes, err := UUIDToBytes(agentID)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, SignPayloadSize)
	copy(payload[0:32], nonce)
	copy(payload[32:48], uuidBytes)
	binary.BigEndian.PutUint64(payload[48:56], uint64(ts)) // #nosec G115 -- epoch timestamp encoded to a fixed 8-byte wire field
	return payload, nil
}

// UUIDToBytes decodes a UUID-like hex string (with or without dashes) into 16 raw bytes.
func UUIDToBytes(s string) ([]byte, error) {
	clean := strings.ReplaceAll(s, "-", "")
	if len(clean) != 32 {
		return nil, ErrInvalidUUID
	}
	return hex.DecodeString(clean)
}

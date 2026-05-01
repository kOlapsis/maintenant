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
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/agent"
	"github.com/kolapsis/maintenant/internal/agentpb"
)

const testAgentUUID = "550e8400-e29b-41d4-a716-446655440000"

// buildPayload constructs the byte-exact signing payload:
// nonce(32) || uuid_bytes(16) || timestamp_be64(8).
func buildPayload(nonce []byte, agentUUID string, ts int64) []byte {
	clean := strings.ReplaceAll(agentUUID, "-", "")
	uuidBytes, _ := hex.DecodeString(clean)

	payload := make([]byte, 56)
	copy(payload[0:32], nonce)
	copy(payload[32:48], uuidBytes)
	binary.BigEndian.PutUint64(payload[48:56], uint64(ts))
	return payload
}

// newTestKeypair generates a fresh Ed25519 keypair for use in tests.
func newTestKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

// newActiveAgent builds an agent.Agent with the given public key and status "active".
func newActiveAgent(pub ed25519.PublicKey) *agent.Agent {
	return &agent.Agent{
		AgentID:   testAgentUUID,
		PublicKey: []byte(pub),
		Status:    "active",
	}
}

// signedAuthResponse builds an AuthResponse signed with the provided private key.
func signedAuthResponse(t *testing.T, priv ed25519.PrivateKey, nonce []byte, agentUUID string, ts int64) *agentpb.AuthResponse {
	t.Helper()
	payload := buildPayload(nonce, agentUUID, ts)
	sig := ed25519.Sign(priv, payload)
	return &agentpb.AuthResponse{
		AgentId:   agentUUID,
		Timestamp: ts,
		Signature: sig,
	}
}

func TestVerify_ValidSignatureActiveAgent(t *testing.T) {
	pub, priv := newTestKeypair(t)
	ag := newActiveAgent(pub)

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	serverNow := time.Now()
	req := signedAuthResponse(t, priv, nonce, testAgentUUID, serverNow.Unix())

	err = Verify(req, nonce, ag, serverNow)

	assert.NoError(t, err)
}

func TestVerify_ClockSkewExceeds300s(t *testing.T) {
	pub, priv := newTestKeypair(t)
	ag := newActiveAgent(pub)

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	serverNow := time.Now()
	// Client timestamp is 301 seconds in the past — exceeds the 300s window.
	clientTs := serverNow.Unix() - 301
	req := signedAuthResponse(t, priv, nonce, testAgentUUID, clientTs)

	err = Verify(req, nonce, ag, serverNow)

	assert.ErrorIs(t, err, ErrClockSkew)
}

func TestVerify_TamperedSignature(t *testing.T) {
	pub, priv := newTestKeypair(t)
	ag := newActiveAgent(pub)

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	serverNow := time.Now()
	req := signedAuthResponse(t, priv, nonce, testAgentUUID, serverNow.Unix())

	// Flip the first byte of the signature to invalidate it.
	req.Signature[0] ^= 0xFF

	err = Verify(req, nonce, ag, serverNow)

	assert.ErrorIs(t, err, ErrBadSignature)
}

func TestVerify_RevokedAgent(t *testing.T) {
	pub, priv := newTestKeypair(t)
	ag := newActiveAgent(pub)
	ag.Status = "revoked"

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	serverNow := time.Now()
	req := signedAuthResponse(t, priv, nonce, testAgentUUID, serverNow.Unix())

	err = Verify(req, nonce, ag, serverNow)

	assert.ErrorIs(t, err, ErrAgentRevoked)
}

func TestVerify_NilAgentReturnsErrAgentUnknown(t *testing.T) {
	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	req := &agentpb.AuthResponse{
		AgentId:   testAgentUUID,
		Timestamp: time.Now().Unix(),
		Signature: make([]byte, ed25519.SignatureSize),
	}

	err = Verify(req, nonce, nil, time.Now())

	assert.ErrorIs(t, err, ErrAgentUnknown)
}

func TestVerify_ClockSkewExactlyAtBoundary(t *testing.T) {
	pub, priv := newTestKeypair(t)
	ag := newActiveAgent(pub)

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	serverNow := time.Now()
	// Exactly 300s skew is at the boundary — should still be accepted.
	clientTs := serverNow.Unix() - 300
	req := signedAuthResponse(t, priv, nonce, testAgentUUID, clientTs)

	err = Verify(req, nonce, ag, serverNow)

	assert.NoError(t, err)
}

func TestVerify_ClockSkewFutureTimestamp(t *testing.T) {
	pub, priv := newTestKeypair(t)
	ag := newActiveAgent(pub)

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	serverNow := time.Now()
	// Client timestamp is 301 seconds in the future.
	clientTs := serverNow.Unix() + 301
	req := signedAuthResponse(t, priv, nonce, testAgentUUID, clientTs)

	err = Verify(req, nonce, ag, serverNow)

	assert.ErrorIs(t, err, ErrClockSkew)
}

func TestVerify_WrongNonceProducesErrBadSignature(t *testing.T) {
	pub, priv := newTestKeypair(t)
	ag := newActiveAgent(pub)

	nonce := make([]byte, 32)
	_, err := rand.Read(nonce)
	require.NoError(t, err)

	serverNow := time.Now()
	req := signedAuthResponse(t, priv, nonce, testAgentUUID, serverNow.Unix())

	// Present a different nonce to the verifier — signature will not match.
	differentNonce := make([]byte, 32)
	_, err = rand.Read(differentNonce)
	require.NoError(t, err)

	err = Verify(req, differentNonce, ag, serverNow)

	assert.ErrorIs(t, err, ErrBadSignature)
}

func TestGenerateChallenge_NonceIs32Bytes(t *testing.T) {
	ch, err := GenerateChallenge()

	require.NoError(t, err)
	assert.Len(t, ch.GetNonce(), 32)
}

func TestGenerateChallenge_TwoCallsProduceDifferentNonces(t *testing.T) {
	ch1, err := GenerateChallenge()
	require.NoError(t, err)

	ch2, err := GenerateChallenge()
	require.NoError(t, err)

	assert.NotEqual(t, ch1.GetNonce(), ch2.GetNonce())
}

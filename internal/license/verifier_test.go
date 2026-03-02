package license

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func signPayload(t *testing.T, priv ed25519.PrivateKey, payload interface{}) SignedResponse {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	sig := ed25519.Sign(priv, data)
	return SignedResponse{
		Payload:   string(data),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}
}

func TestVerify_ValidSignature(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	payload := LicensePayload{
		Status:     "active",
		Plan:       "pro",
		Features:   []string{"all"},
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour),
		VerifiedAt: time.Now(),
	}

	signed := signPayload(t, priv, payload)
	result, err := verify(pub, signed)

	require.NoError(t, err)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, "pro", result.Plan)
}

func TestVerify_WrongKey(t *testing.T) {
	_, priv := generateTestKeyPair(t)
	otherPub, _ := generateTestKeyPair(t)

	payload := LicensePayload{
		Status: "active",
		Plan:   "pro",
	}

	signed := signPayload(t, priv, payload)
	_, err := verify(otherPub, signed)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid license signature")
}

func TestVerify_TamperedPayload(t *testing.T) {
	pub, priv := generateTestKeyPair(t)

	payload := LicensePayload{
		Status: "active",
		Plan:   "pro",
	}

	signed := signPayload(t, priv, payload)
	// Tamper with the payload
	signed.Payload = `{"status":"active","plan":"enterprise","features":null,"expires_at":"0001-01-01T00:00:00Z","verified_at":"0001-01-01T00:00:00Z"}`

	_, err := verify(pub, signed)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid license signature")
}

func TestVerify_InvalidSignatureEncoding(t *testing.T) {
	pub, _ := generateTestKeyPair(t)

	signed := SignedResponse{
		Payload:   `{"status":"active"}`,
		Signature: "not-valid-base64!!!",
	}

	_, err := verify(pub, signed)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature encoding")
}

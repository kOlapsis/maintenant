package license

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_WriteAndRead(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	dir := t.TempDir()

	payload := LicensePayload{
		Status:     "active",
		Plan:       "pro",
		Features:   []string{"all"},
		ExpiresAt:  time.Now().Add(30 * 24 * time.Hour).Truncate(time.Second),
		VerifiedAt: time.Now().Truncate(time.Second),
	}

	signed := signPayload(t, priv, payload)
	err := writeCache(dir, &signed)
	require.NoError(t, err)

	// Verify file was written
	_, err = os.Stat(filepath.Join(dir, cacheFileName))
	require.NoError(t, err)

	// Read back
	result, err := readCache(dir, pub)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "active", result.Status)
	assert.Equal(t, "pro", result.Plan)
}

func TestCache_MissingFile(t *testing.T) {
	pub, _ := generateTestKeyPair(t)
	dir := t.TempDir()

	result, err := readCache(dir, pub)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestCache_CorruptedFile(t *testing.T) {
	pub, _ := generateTestKeyPair(t)
	dir := t.TempDir()

	err := os.WriteFile(filepath.Join(dir, cacheFileName), []byte("not json"), 0600)
	require.NoError(t, err)

	result, err := readCache(dir, pub)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "parsing license cache")
}

func TestCache_TamperedPayload(t *testing.T) {
	pub, priv := generateTestKeyPair(t)
	dir := t.TempDir()

	payload := LicensePayload{
		Status: "active",
		Plan:   "pro",
	}

	signed := signPayload(t, priv, payload)
	// Write the cache
	err := writeCache(dir, &signed)
	require.NoError(t, err)

	// Tamper with the cached file
	data, err := os.ReadFile(filepath.Join(dir, cacheFileName))
	require.NoError(t, err)
	var cached SignedResponse
	require.NoError(t, json.Unmarshal(data, &cached))
	cached.Payload = `{"status":"active","plan":"enterprise"}`
	tampered, err := json.Marshal(cached)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, cacheFileName), tampered, 0600))

	// Read should fail signature check
	result, err := readCache(dir, pub)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "verifying cached license")
}

func TestCache_Delete(t *testing.T) {
	_, priv := generateTestKeyPair(t)
	dir := t.TempDir()

	payload := LicensePayload{Status: "active", Plan: "pro"}
	signed := signPayload(t, priv, payload)
	require.NoError(t, writeCache(dir, &signed))

	// File exists
	_, err := os.Stat(filepath.Join(dir, cacheFileName))
	require.NoError(t, err)

	deleteCache(dir)

	// File removed
	_, err = os.Stat(filepath.Join(dir, cacheFileName))
	assert.True(t, os.IsNotExist(err))
}

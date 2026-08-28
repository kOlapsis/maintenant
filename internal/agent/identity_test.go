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
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadOrCreate_Creates(t *testing.T) {
	dir := t.TempDir()

	id, err := LoadOrCreate(dir)
	require.NoError(t, err)
	assert.NotEmpty(t, id.AgentID)
	assert.Len(t, id.PublicKey, ed25519.PublicKeySize)
	assert.Len(t, id.PrivateKey, ed25519.PrivateKeySize)
	assert.False(t, id.Registered)
}

func TestLoadOrCreate_FilePerms(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadOrCreate(dir)
	require.NoError(t, err)

	info, err := os.Stat(filepath.Join(dir, identityFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestLoadOrCreate_ReloadsExisting(t *testing.T) {
	dir := t.TempDir()

	id1, err := LoadOrCreate(dir)
	require.NoError(t, err)

	id2, err := LoadOrCreate(dir)
	require.NoError(t, err)

	assert.Equal(t, id1.AgentID, id2.AgentID)
	assert.Equal(t, id1.PublicKey, id2.PublicKey)
}

func TestIdentity_Save(t *testing.T) {
	dir := t.TempDir()

	id, err := LoadOrCreate(dir)
	require.NoError(t, err)
	assert.False(t, id.Registered)

	id.Registered = true
	require.NoError(t, id.Save(dir))

	id2, err := LoadOrCreate(dir)
	require.NoError(t, err)
	assert.True(t, id2.Registered)
}

func TestIdentity_Sign(t *testing.T) {
	dir := t.TempDir()
	id, err := LoadOrCreate(dir)
	require.NoError(t, err)

	msg := []byte("test-nonce-payload")
	sig := id.Sign(msg)

	ok := ed25519.Verify(ed25519.PublicKey(id.PublicKey), msg, sig)
	assert.True(t, ok)
}

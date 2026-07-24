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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const identityFile = "identity.json"

// Identity holds the persistent agent identity stored on disk.
type Identity struct {
	AgentID      string     `json:"agent_id"`
	PublicKey    []byte     `json:"public_key"`
	PrivateKey   []byte     `json:"private_key"`
	Registered   bool       `json:"registered"`
	RegisteredAt *time.Time `json:"registered_at,omitempty"`
}

// LoadOrCreate loads an existing identity from dataDir/identity.json, or generates and
// persists a new Ed25519 keypair if no file exists. File is created with mode 0600.
func LoadOrCreate(dataDir string) (*Identity, error) {
	path := filepath.Join(dataDir, identityFile)

	data, err := os.ReadFile(path) // #nosec G304 -- path is dataDir + constant identity filename, not user input
	if err == nil {
		var id Identity
		if jsonErr := json.Unmarshal(data, &id); jsonErr != nil {
			return nil, fmt.Errorf("parse identity file: %w", jsonErr)
		}
		return &id, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read identity file: %w", err)
	}

	// Generate new keypair
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ed25519 key: %w", err)
	}

	id := &Identity{
		AgentID:    generateAgentID(),
		PublicKey:  []byte(pub),
		PrivateKey: []byte(priv),
	}

	encoded, err := json.Marshal(id) // #nosec G117 -- agent identity file persists its own Ed25519 private key by design
	if err != nil {
		return nil, fmt.Errorf("marshal identity: %w", err)
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600) // #nosec G304 -- path is dataDir + constant identity filename, not user input
	if err != nil {
		return nil, fmt.Errorf("create identity file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.Write(encoded); err != nil {
		return nil, fmt.Errorf("write identity file: %w", err)
	}

	// Defensive chmod in case umask overrode the creation mode
	if err := os.Chmod(path, 0600); err != nil {
		return nil, fmt.Errorf("chmod identity file: %w", err)
	}

	return id, nil
}

// Save persists updated identity fields (e.g. Registered) back to disk with mode 0600.
func (id *Identity) Save(dataDir string) error {
	path := filepath.Join(dataDir, identityFile)
	encoded, err := json.Marshal(id) // #nosec G117 -- agent identity file persists its own Ed25519 private key by design
	if err != nil {
		return fmt.Errorf("marshal identity: %w", err)
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		return fmt.Errorf("write identity file: %w", err)
	}
	return nil
}

// Sign returns an Ed25519 signature over message using the stored private key.
func (id *Identity) Sign(message []byte) []byte {
	return ed25519.Sign(ed25519.PrivateKey(id.PrivateKey), message)
}

func generateAgentID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

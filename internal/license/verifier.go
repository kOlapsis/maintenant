// Copyright 2026 Benjamin Touchard (kOlapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/kolapsis/maintenant/internal/extension"
)

const (
	licenseServerURL = "https://license.maintenant.dev"
)

// licenseServerOverride can be set via -ldflags to point to a dev/staging server.
var licenseServerOverride string

func getLicenseServerURL() string {
	if licenseServerOverride != "" {
		return licenseServerOverride
	}
	return licenseServerURL
}

// SignedResponse is the raw response from the license server.
type SignedResponse struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// ServerError is returned by fetchLicense when the server responds with a
// non-200 status and a parseable error body. It carries the HTTP status code,
// the server-provided status string, and the human-readable message.
type ServerError struct {
	HTTPStatus int
	Status     string
	Message    string
}

func (e *ServerError) Error() string { return e.Message }

// LicensePayload is the decoded and verified license payload.
//
// Edition is optional: licenses issued before the three-edition model carry no
// such field, and ResolveEdition falls back for them. Features is accepted and
// ignored — capabilities come from the edition, never from the server.
type LicensePayload struct {
	Status     string    `json:"status"`
	Edition    string    `json:"edition"`
	Plan       string    `json:"plan"`
	Features   []string  `json:"features"`
	ExpiresAt  time.Time `json:"expires_at"`
	VerifiedAt time.Time `json:"verified_at"`
	Message    string    `json:"message"`
}

// ResolveEdition derives the edition a verified payload grants, in the order
// fixed by contracts/license-payload.md:
//
//  1. edition present and recognised → that edition
//  2. edition present but unknown    → community, and the discrepancy is logged
//  3. edition absent, plan=personal  → personal
//  4. edition absent, status usable  → pro
//  5. nothing usable                 → community
//
// Rule 4 is the compatibility clause: the license server returns plan "pro" for
// every client today, so without it every license in service would lose its
// capabilities on the first restart after the update. Rule 2 cannot be confused
// with it — rule 2 requires the field to be present.
func ResolveEdition(p *LicensePayload, logger *slog.Logger) extension.Edition {
	// Rule 5 first: nothing is granted by a license that is not in force. An
	// expired or revoked payload grants Community whatever it declares.
	if p == nil || (p.Status != "active" && p.Status != "grace") {
		return extension.Community
	}

	// Rules 1 and 2.
	if p.Edition != "" {
		edition, ok := extension.ParseEdition(p.Edition)
		if !ok && logger != nil {
			logger.Warn("license declares an edition this build does not know, falling back to Community",
				"declared_edition", p.Edition,
			)
		}
		return edition
	}

	// Rule 3.
	if p.Plan == string(extension.Personal) {
		return extension.Personal
	}

	// Rule 4 — the compatibility clause.
	return extension.Pro
}

// verify checks the Ed25519 signature on a SignedResponse, then decodes the
// payload. Returns the parsed payload or an error if the signature is invalid.
func verify(publicKey ed25519.PublicKey, resp SignedResponse) (*LicensePayload, error) {
	sig, err := base64.StdEncoding.DecodeString(resp.Signature)
	if err != nil {
		return nil, fmt.Errorf("invalid signature encoding: %w", err)
	}

	if !ed25519.Verify(publicKey, []byte(resp.Payload), sig) {
		return nil, fmt.Errorf("invalid license signature")
	}

	var payload LicensePayload
	if err := json.Unmarshal([]byte(resp.Payload), &payload); err != nil {
		return nil, fmt.Errorf("invalid license payload: %w", err)
	}

	return &payload, nil
}

// fetchLicense calls the license server with the given key and returns the
// signed response. The caller is responsible for verifying the signature.
func fetchLicense(ctx context.Context, client *http.Client, serverURL, licenseKey, version string) (*SignedResponse, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/license/verify", serverURL), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating license request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+licenseKey)
	req.Header.Set("User-Agent", "maintenant/"+version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("license server request failed: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading license response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var p LicensePayload
		var signed SignedResponse
		if json.Unmarshal(body, &signed) == nil && signed.Payload != "" {
			_ = json.Unmarshal([]byte(signed.Payload), &p)
		} else {
			_ = json.Unmarshal(body, &p)
		}
		if p.Message != "" {
			return nil, resp.StatusCode, &ServerError{HTTPStatus: resp.StatusCode, Status: p.Status, Message: p.Message}
		}
		return nil, resp.StatusCode, fmt.Errorf("license server returned HTTP %d", resp.StatusCode)
	}

	var signed SignedResponse
	if err := json.Unmarshal(body, &signed); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decoding license response: %w", err)
	}

	return &signed, resp.StatusCode, nil
}

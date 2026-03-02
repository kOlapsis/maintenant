package license

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	licenseServerURL = "https://license.maintenant.io"
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

// LicensePayload is the decoded and verified license payload.
type LicensePayload struct {
	Status     string    `json:"status"`
	Plan       string    `json:"plan"`
	Features   []string  `json:"features"`
	ExpiresAt  time.Time `json:"expires_at"`
	VerifiedAt time.Time `json:"verified_at"`
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/v1/license/verify", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("creating license request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+licenseKey)
	req.Header.Set("User-Agent", "maintenant/"+version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("license server request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading license response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, fmt.Errorf("license server returned HTTP %d", resp.StatusCode)
	}

	var signed SignedResponse
	if err := json.Unmarshal(body, &signed); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decoding license response: %w", err)
	}

	return &signed, resp.StatusCode, nil
}

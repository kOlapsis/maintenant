// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/update"
)

// Acceptance tests of 047-audit-small-fixes, C13: a changelog source is a web URL or nothing.

// pushLabelledImage stores an image carrying labels in the registry at host and returns its reference.
func pushLabelledImage(t *testing.T, host, repo string, labels map[string]string) string {
	t.Helper()
	img, err := random.Image(64, 1)
	require.NoError(t, err)
	cf, err := img.ConfigFile()
	require.NoError(t, err)
	cf = cf.DeepCopy()
	cf.Config.Labels = labels
	img, err = mutate.ConfigFile(img, cf)
	require.NoError(t, err)

	ref := host + "/" + repo + ":1.0"
	parsed, err := name.ParseReference(ref)
	require.NoError(t, err)
	require.NoError(t, remote.Write(parsed, img))
	return ref
}

// FR-006, US1 scenario 4: only http:// and https:// sources are kept; javascript:, file: and the rest are dropped.
func TestResolveChangelog_KeepsOnlyWebSourceURLs(t *testing.T) {
	srv := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(srv.Close)
	host := strings.TrimPrefix(srv.URL, "http://")
	cr := NewChangelogResolver(update.NewRegistryClient(), slog.New(slog.NewTextHandler(io.Discard, nil)))

	for i, tc := range []struct {
		label, source, want string
	}{
		{"org.opencontainers.image.source", "javascript:alert(1)", ""},
		{"org.opencontainers.image.source", "file:///etc/passwd", ""},
		{"org.opencontainers.image.source", "data:text/html,<script>alert(1)</script>", ""},
		{"org.opencontainers.image.source", "ftp://files.example.com/app", ""},
		{"org.opencontainers.image.source", "HTTPS://gitlab.example.com/acme/app", ""},
		{"org.label-schema.vcs-url", "javascript:void(0)", ""},
		{"org.opencontainers.image.source", "http://git.example.com/acme/app", "http://git.example.com/acme/app"},
		{"org.opencontainers.image.source", "https://gitlab.example.com/acme/app.git", "https://gitlab.example.com/acme/app.git"},
		{"org.label-schema.vcs-url", "https://codeberg.org/acme/tool", "https://codeberg.org/acme/tool"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			ref := pushLabelledImage(t, host, fmt.Sprintf("acme/app-%d", i), map[string]string{tc.label: tc.source})

			changelogURL, summary, breaking, sourceURL := cr.ResolveChangelog(context.Background(), ref, "1.0")

			assert.Equal(t, tc.want, sourceURL)
			assert.Empty(t, changelogURL)
			assert.Empty(t, summary)
			assert.False(t, breaking)
		})
	}
}

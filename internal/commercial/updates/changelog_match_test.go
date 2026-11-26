// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/stretchr/testify/assert"

	"github.com/kolapsis/maintenant/internal/update"
)

func TestResolveChangelog_ShowsOnlyTheReleaseOfTheTargetTag(t *testing.T) {
	reg := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(reg.Close)
	ref := pushLabelledImage(t, strings.TrimPrefix(reg.URL, "http://"), "acme/app",
		map[string]string{"org.opencontainers.image.source": "https://github.com/acme/app"})

	github := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/repos/acme/app/releases", r.URL.Path)
		writeJSON(t, w, []ghRelease{
			{TagName: "v2.0.0-rc1", Body: "BREAKING CHANGE: new config format", HTMLURL: "https://github.com/acme/app/releases/v2.0.0-rc1"},
			{TagName: "v1.0", Body: "Fixes a crash at startup", HTMLURL: "https://github.com/acme/app/releases/v1.0"},
		})
	}))
	t.Cleanup(github.Close)

	cr := NewChangelogResolver(update.NewRegistryClient(), testLogger())
	cr.apiURL = github.URL

	changelogURL, summary, breaking, sourceURL := cr.ResolveChangelog(context.Background(), ref, "1.0")
	assert.Equal(t, "https://github.com/acme/app/releases/v1.0", changelogURL)
	assert.Equal(t, "Fixes a crash at startup", summary)
	assert.False(t, breaking)
	assert.Equal(t, "https://github.com/acme/app", sourceURL)

	changelogURL, summary, breaking, sourceURL = cr.ResolveChangelog(context.Background(), ref, "1.1")
	assert.Empty(t, changelogURL, "the notes of another release are not those of the update")
	assert.Empty(t, summary)
	assert.False(t, breaking)
	assert.Equal(t, "https://github.com/acme/app", sourceURL)
}

func TestDetectBreakingChanges_IgnoresOrdinaryReleaseNoteWords(t *testing.T) {
	assert.False(t, DetectBreakingChanges("Removed a duplicated log line. The --old flag is deprecated."))
	assert.True(t, DetectBreakingChanges("Breaking change: the config file moved"))
}

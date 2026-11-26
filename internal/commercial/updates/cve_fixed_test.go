// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See internal/commercial/LICENSE.

package updates

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFixedInFor_TakesTheRangeOfTheRunningVersion(t *testing.T) {
	pkg := osvPackage{Ecosystem: "Go", Name: "github.com/traefik/traefik/v3"}
	rec := &osvVulnRecord{Affected: []osvAffected{
		{Package: osvPackage{Ecosystem: "Go", Name: "github.com/traefik/traefik/v2"}, Ranges: []osvRange{
			{Type: "SEMVER", Events: []osvEvent{{Introduced: "0"}, {Fixed: "2.11.53"}}},
		}},
		{Package: pkg, Ranges: []osvRange{
			{Type: "GIT", Events: []osvEvent{{Introduced: "0"}, {Fixed: "3f2a9c1"}}},
			{Type: "SEMVER", Events: []osvEvent{{Introduced: "0"}, {Fixed: "3.6.24"}, {Introduced: "3.7.0"}, {Fixed: "3.7.9"}}},
		}},
	}}
	query := func(version string) ImageCVEQuery {
		return ImageCVEQuery{PackageName: pkg.Name, Ecosystem: pkg.Ecosystem, Version: version}
	}

	assert.Equal(t, "3.6.24", fixedInFor(query("3.1.0"), rec))
	assert.Equal(t, "3.7.9", fixedInFor(query("3.7.2"), rec))
	assert.Empty(t, fixedInFor(query("3.6.30"), rec), "between two affected ranges")
	assert.Empty(t, fixedInFor(query("latest"), rec))
}

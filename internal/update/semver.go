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

package update

import (
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// knownVariantSuffixes lists OS/distro suffixes that are NOT semver prereleases.
// Ordered longest-first so "-slim-bookworm" matches before "-bookworm".
var knownVariantSuffixes = []string{
	"-slim-bookworm",
	"-slim-bullseye",
	"-slim-buster",
	"-alpine3.21",
	"-alpine3.20",
	"-alpine3.19",
	"-alpine3.18",
	"-alpine",
	"-bookworm",
	"-bullseye",
	"-buster",
	"-noble",
	"-jammy",
	"-focal",
}

// splitVariant separates a Docker tag into its version part and variant suffix.
// e.g. "18.3-alpine" → ("18.3", "-alpine"), "3.19.1" → ("3.19.1", "")
func splitVariant(tag string) (version, variant string) {
	lower := strings.ToLower(tag)
	for _, suffix := range knownVariantSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return tag[:len(tag)-len(suffix)], tag[len(tag)-len(suffix):]
		}
	}
	return tag, ""
}

// ParseTag attempts to parse a Docker tag as a semver version.
// Returns nil, error for non-semver tags like "latest", "alpine".
func ParseTag(tag string) (*semver.Version, error) {
	v, err := semver.NewVersion(tag)
	if err != nil {
		return nil, err
	}
	return v, nil
}

// ClassifyUpdate determines the type of version bump between two versions.
func ClassifyUpdate(current, latest *semver.Version) UpdateType {
	if current == nil || latest == nil {
		return UpdateTypeUnknown
	}
	if latest.Major() > current.Major() {
		return UpdateTypeMajor
	}
	if latest.Minor() > current.Minor() {
		return UpdateTypeMinor
	}
	if latest.Patch() > current.Patch() {
		return UpdateTypePatch
	}
	return UpdateTypeUnknown
}

// tagVersion pairs a parsed semver version with its original tag string.
type tagVersion struct {
	original string
	version  *semver.Version
}

// sortTagVersions filters tags to those matching the given variant suffix,
// parses them as semver, skips prereleases, and returns them sorted ascending.
// requireDots, when true, skips candidates whose version part has no dots —
// this filters out pure numeric build IDs (e.g. "608111629") that Masterminds/semver
// accepts as valid single-component versions but are not real release tags.
func sortTagVersions(tags []string, variant string, requireDots bool) []tagVersion {
	var result []tagVersion
	for _, tag := range tags {
		_, tv := splitVariant(tag)
		if !strings.EqualFold(tv, variant) {
			continue
		}
		versionPart, _ := splitVariant(tag)
		if requireDots && !strings.Contains(strings.TrimPrefix(versionPart, "v"), ".") {
			continue
		}
		v, err := semver.NewVersion(versionPart)
		if err != nil {
			continue
		}
		if v.Prerelease() != "" {
			continue
		}
		result = append(result, tagVersion{original: tag, version: v})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].version.LessThan(result[j].version)
	})
	return result
}

// SortTags filters non-semver tags and returns sorted semver versions (ascending).
func SortTags(tags []string) []*semver.Version {
	var versions []*semver.Version
	for _, tag := range tags {
		v, err := semver.NewVersion(tag)
		if err != nil {
			continue
		}
		// Skip pre-release versions
		if v.Prerelease() != "" {
			continue
		}
		versions = append(versions, v)
	}
	sort.Sort(semver.Collection(versions))
	return versions
}

// FindBestUpdate finds the best available update for the given current tag among all tags.
// For semver tags: finds the highest version with the same variant suffix (e.g. -alpine).
// For non-semver tags: returns the latest tag if digests differ (digest_only mode).
func FindBestUpdate(currentTag string, allTags []string) (bestTag string, updateType UpdateType) {
	versionPart, variant := splitVariant(currentTag)

	currentVer, err := semver.NewVersion(versionPart)
	if err != nil {
		// Non-semver tag (e.g. "lts", "alpine", "stable", "latest"):
		// return the same tag so the scanner can do digest-only comparison.
		// Never suggest switching to a different channel like "latest".
		for _, t := range allTags {
			if t == currentTag {
				return currentTag, UpdateTypeDigestOnly
			}
		}
		return "", UpdateTypeUnknown
	}

	// Determine whether the current version part uses dots (e.g. "1.20.1", "v1.20.1").
	// Tags without dots (e.g. "608111629") are build IDs or timestamps that happen to be
	// valid single-component semver — they must be excluded when the current tag is dotted.
	currentHasDots := strings.Contains(strings.TrimPrefix(versionPart, "v"), ".")

	candidates := sortTagVersions(allTags, variant, currentHasDots)
	if len(candidates) == 0 {
		return "", UpdateTypeUnknown
	}

	// Find the highest version greater than current
	var best *tagVersion
	for i := range candidates {
		if candidates[i].version.GreaterThan(currentVer) {
			best = &candidates[i]
		}
	}

	if best == nil {
		return "", UpdateTypeUnknown
	}

	return best.original, ClassifyUpdate(currentVer, best.version)
}

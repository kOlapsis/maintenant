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
	"slices"
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

// semverPrecision counts the numeric components a version part is written with:
// "v3" → 1, "1.2" → 2, "1.20.1" → 3, "1.2-rc1" → 2.
// Anything below 3 is a floating tag: the registry moves it to the newest release
// of that line, so the container already runs whatever it currently resolves to.
func semverPrecision(versionPart string) int {
	core := strings.TrimPrefix(versionPart, "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	return strings.Count(core, ".") + 1
}

// hasVPrefix reports whether a version part is written with a "v" prefix ("v3" vs "3").
// A repository sticks to one form; the other is an alias at best.
func hasVPrefix(versionPart string) bool {
	return strings.HasPrefix(versionPart, "v")
}

// digitCount returns the number of decimal digits in a version component.
func digitCount(n uint64) int {
	count := 1
	for n >= 10 {
		n /= 10
		count++
	}
	return count
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
// precision, when non-zero, keeps only tags written with that many numeric components.
func sortTagVersions(tags []string, variant string, requireDots bool, precision int) []tagVersion {
	var result []tagVersion
	for _, tag := range tags {
		versionPart, tv := splitVariant(tag)
		if !strings.EqualFold(tv, variant) {
			continue
		}
		if requireDots && !strings.Contains(strings.TrimPrefix(versionPart, "v"), ".") {
			continue
		}
		if precision > 0 && semverPrecision(versionPart) != precision {
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

// digestOnly returns the current tag for digest comparison when the registry still
// publishes it, so a republished or moved tag is detected. It never switches channel.
func digestOnly(currentTag string, allTags []string) (string, UpdateType) {
	if slices.Contains(allTags, currentTag) {
		return currentTag, UpdateTypeDigestOnly
	}
	return "", UpdateTypeUnknown
}

// bestFloatingUpdate returns the highest tag strictly newer than the current one and
// written the same way: same variant suffix, same "v" prefix, same number of numeric
// components. Candidates whose major has far more digits than the current one are
// build IDs (e.g. "608111629"), not releases.
func bestFloatingUpdate(currentVer *semver.Version, versionPart, variant string, allTags []string) *tagVersion {
	candidates := sortTagVersions(allTags, variant, false, semverPrecision(versionPart))

	var best *tagVersion
	for i := range candidates {
		c := &candidates[i]
		candidatePart, _ := splitVariant(c.original)
		if hasVPrefix(candidatePart) != hasVPrefix(versionPart) {
			continue
		}
		if digitCount(c.version.Major()) > digitCount(currentVer.Major())+1 {
			continue
		}
		if c.version.GreaterThan(currentVer) {
			best = c
		}
	}
	return best
}

// FindBestUpdate finds the best available update for the given current tag among all tags.
// For pinned semver tags (major.minor.patch): finds the highest version with the same
// variant suffix (e.g. -alpine).
// For floating tags (non-semver channels like "latest" or "lts", and partial versions
// like "v3" or "1.2"), the registry already moves the tag to the newest release of that
// line, so a more precise tag underneath it is what the container runs, not an update.
// Only a higher tag of the same shape counts; otherwise the same tag is returned so the
// scanner compares digests.
func FindBestUpdate(currentTag string, allTags []string) (bestTag string, updateType UpdateType) {
	versionPart, variant := splitVariant(currentTag)

	currentVer, err := semver.NewVersion(versionPart)
	if err != nil {
		// Non-semver tag (e.g. "lts", "alpine", "stable", "latest").
		return digestOnly(currentTag, allTags)
	}

	// Partial version tag (e.g. "v3", "1.2", "16-bookworm"): floating channel.
	if semverPrecision(versionPart) < 3 {
		best := bestFloatingUpdate(currentVer, versionPart, variant, allTags)
		if best == nil {
			return digestOnly(currentTag, allTags)
		}
		return best.original, ClassifyUpdate(currentVer, best.version)
	}

	// Tags without dots (e.g. "608111629") are build IDs or timestamps that happen to be
	// valid single-component semver — they must be excluded when the current tag is dotted.
	candidates := sortTagVersions(allTags, variant, true, 0)
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

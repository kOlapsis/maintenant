package update

import (
	"sort"

	"github.com/Masterminds/semver/v3"
)

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
// For semver tags: finds the highest version within the same major.
// For non-semver tags: returns the latest tag if digests differ (digest_only mode).
func FindBestUpdate(currentTag string, allTags []string) (bestTag string, updateType UpdateType) {
	currentVer, err := semver.NewVersion(currentTag)
	if err != nil {
		// Non-semver tag: return "latest" if available, mark as digest_only
		for _, t := range allTags {
			if t == "latest" {
				return "latest", UpdateTypeDigestOnly
			}
		}
		return "", UpdateTypeUnknown
	}

	versions := SortTags(allTags)
	if len(versions) == 0 {
		return "", UpdateTypeUnknown
	}

	// Find the highest version greater than current
	var best *semver.Version
	for _, v := range versions {
		if v.GreaterThan(currentVer) {
			best = v
		}
	}

	if best == nil {
		return "", UpdateTypeUnknown
	}

	return best.Original(), ClassifyUpdate(currentVer, best)
}

// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package container

import "strings"

// ApplyImageLabels fills the image metadata from OCI or label-schema labels and reports whether it changed.
func (c *Container) ApplyImageLabels(labels map[string]string) bool {
	version := firstLabel(labels, false, "org.opencontainers.image.version", "org.label-schema.version")
	source := firstLabel(labels, true, "org.opencontainers.image.source", "org.label-schema.vcs-url")
	url := firstLabel(labels, true, "org.opencontainers.image.url", "org.label-schema.url", "org.opencontainers.image.documentation")
	description := firstLabel(labels, false, "org.opencontainers.image.description", "org.label-schema.description")

	changed := c.ImageVersion != version || c.ImageSource != source || c.ImageURL != url || c.ImageDescription != description
	c.ImageVersion, c.ImageSource, c.ImageURL, c.ImageDescription = version, source, url, description
	return changed
}

func firstLabel(labels map[string]string, webOnly bool, keys ...string) string {
	for _, k := range keys {
		v := strings.TrimSpace(labels[k])
		if v == "" {
			continue
		}
		if webOnly {
			lower := strings.ToLower(v)
			if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
				continue
			}
		}
		return v
	}
	return ""
}

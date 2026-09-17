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

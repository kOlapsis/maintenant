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

package hoststat

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
)

// Source values of an OSRelease.
const (
	OSSourceHostFile       = "host_file"
	OSSourceKubernetesNode = "kubernetes_node"
)

// Reasons why an OSRelease carries no identity.
const (
	OSReasonMountMissing   = "mount_missing"
	OSReasonFileUnreadable = "file_unreadable"
	OSReasonNodeNotFound   = "node_not_found"
)

var (
	// hostOSReleasePath is where the host os-release is mounted inside the
	// container. Deploy with: -v /etc/os-release:/host/etc/os-release:ro
	hostOSReleasePath = "/host/etc/os-release"
	osReleasePath     = "/etc/os-release"

	containerMarkers = []string{"/.dockerenv", "/run/.containerenv"}
)

// OSRelease identifies the operating system of a host.
type OSRelease struct {
	ID                string
	VersionID         string
	PrettyName        string
	Source            string
	UnavailableReason string
}

// ReadOSRelease returns the host operating system identity, or the reason it
// could not be read.
func ReadOSRelease() OSRelease {
	if st, err := os.Stat(hostOSReleasePath); err == nil {
		if st.IsDir() {
			return OSRelease{UnavailableReason: OSReasonFileUnreadable}
		}
		return readOSReleaseFile(hostOSReleasePath)
	}

	// Inside a container /etc/os-release describes the image, not the host:
	// reading it would report a confidently wrong identity.
	if inContainer() {
		return OSRelease{UnavailableReason: OSReasonMountMissing}
	}
	return readOSReleaseFile(osReleasePath)
}

func readOSReleaseFile(path string) OSRelease {
	f, err := os.Open(path) // #nosec G304 -- path is one of two fixed package constants, not user input
	if err != nil {
		return OSRelease{UnavailableReason: OSReasonFileUnreadable}
	}
	defer func() { _ = f.Close() }()

	if st, err := f.Stat(); err != nil || st.IsDir() {
		return OSRelease{UnavailableReason: OSReasonFileUnreadable}
	}

	rel := parseOSRelease(f)
	if rel.ID == "" {
		return OSRelease{UnavailableReason: OSReasonFileUnreadable}
	}
	rel.Source = OSSourceHostFile
	return rel
}

func inContainer() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("MAINTENANT_CONTAINER"))) {
	case "1", "true", "yes", "on":
		return true
	}
	for _, marker := range containerMarkers {
		if _, err := os.Stat(marker); err == nil {
			return true
		}
	}
	return false
}

// parseOSRelease reads the KEY=value lines defined by the systemd os-release
// specification and keeps ID, VERSION_ID and PRETTY_NAME.
func parseOSRelease(r io.Reader) OSRelease {
	var rel OSRelease

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value := unquoteOSReleaseValue(strings.TrimSpace(raw))

		switch strings.TrimSpace(key) {
		case "ID":
			rel.ID = value
		case "VERSION_ID":
			rel.VersionID = value
		case "PRETTY_NAME":
			rel.PrettyName = value
		}
	}
	return rel
}

func unquoteOSReleaseValue(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if quote != '"' && quote != '\'' || value[len(value)-1] != quote {
		return value
	}
	inner := value[1 : len(value)-1]
	if quote == '\'' {
		return inner
	}

	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' && i+1 < len(inner) {
			switch inner[i+1] {
			case '"', '\\', '$', '`':
				i++
			}
		}
		b.WriteByte(inner[i])
	}
	return b.String()
}

var osImagePatterns = []struct {
	id string
	re *regexp.Regexp
}{
	{"ubuntu", regexp.MustCompile(`^Ubuntu (\d+\.\d+)`)},
	{"debian", regexp.MustCompile(`^Debian GNU/Linux (\d+)`)},
	{"rhel", regexp.MustCompile(`^Red Hat Enterprise Linux (\d+)`)},
	{"rocky", regexp.MustCompile(`^Rocky Linux (\d+)`)},
	{"almalinux", regexp.MustCompile(`^AlmaLinux (\d+)`)},
	{"alpine", regexp.MustCompile(`^Alpine Linux v(\d+\.\d+)`)},
	{"sles", regexp.MustCompile(`^SUSE Linux Enterprise Server (\d+) SP(\d+)`)},
}

// ParseOSImage derives an operating system identity from a Kubernetes node
// osImage string. An unrecognised string keeps its pretty name and no version.
func ParseOSImage(osImage string) OSRelease {
	osImage = strings.TrimSpace(osImage)
	if osImage == "" {
		return OSRelease{Source: OSSourceKubernetesNode, UnavailableReason: OSReasonNodeNotFound}
	}

	for _, p := range osImagePatterns {
		m := p.re.FindStringSubmatch(osImage)
		if m == nil {
			continue
		}
		return OSRelease{
			ID:         p.id,
			VersionID:  strings.Join(m[1:], "."),
			PrettyName: osImage,
			Source:     OSSourceKubernetesNode,
		}
	}

	return OSRelease{PrettyName: osImage, Source: OSSourceKubernetesNode}
}

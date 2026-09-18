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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	ubuntu2204 = `PRETTY_NAME="Ubuntu 22.04.5 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
VERSION="22.04.5 LTS (Jammy Jellyfish)"
VERSION_CODENAME=jammy
ID=ubuntu
ID_LIKE=debian
HOME_URL="https://www.ubuntu.com/"
UBUNTU_CODENAME=jammy
`

	debian12 = `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION="12 (bookworm)"
VERSION_CODENAME=bookworm
ID=debian
HOME_URL="https://www.debian.org/"
`

	debianTesting = `PRETTY_NAME="Debian GNU/Linux trixie/sid"
NAME="Debian GNU/Linux"
VERSION_CODENAME=trixie
ID=debian
HOME_URL="https://www.debian.org/"
`

	alpine320 = `NAME="Alpine Linux"
ID=alpine
VERSION_ID=3.20.10
PRETTY_NAME="Alpine Linux v3.20"
HOME_URL="https://alpinelinux.org/"
`

	rhel94 = `NAME="Red Hat Enterprise Linux"
VERSION="9.4 (Plow)"
ID="rhel"
ID_LIKE="fedora"
VERSION_ID="9.4"
PLATFORM_ID="platform:el9"
PRETTY_NAME="Red Hat Enterprise Linux 9.4 (Plow)"
`

	rocky93 = `NAME="Rocky Linux"
VERSION="9.3 (Blue Onyx)"
ID="rocky"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.3"
PRETTY_NAME="Rocky Linux 9.3 (Blue Onyx)"
`

	alma98 = `NAME="AlmaLinux"
VERSION="9.8 (Olive Jaguar)"
ID="almalinux"
ID_LIKE="rhel centos fedora"
VERSION_ID="9.8"
PRETTY_NAME="AlmaLinux 9.8 (Olive Jaguar)"
`

	sles15sp5 = `NAME="SLES"
VERSION="15-SP5"
VERSION_ID="15.5"
PRETTY_NAME="SUSE Linux Enterprise Server 15 SP5"
ID="sles"
ID_LIKE="suse"
`
)

func TestReadOSReleaseFamilies(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		id         string
		versionID  string
		prettyName string
	}{
		{"ubuntu", ubuntu2204, "ubuntu", "22.04", "Ubuntu 22.04.5 LTS"},
		{"debian", debian12, "debian", "12", "Debian GNU/Linux 12 (bookworm)"},
		{"debian testing", debianTesting, "debian", "", "Debian GNU/Linux trixie/sid"},
		{"alpine", alpine320, "alpine", "3.20.10", "Alpine Linux v3.20"},
		{"rhel", rhel94, "rhel", "9.4", "Red Hat Enterprise Linux 9.4 (Plow)"},
		{"rocky", rocky93, "rocky", "9.3", "Rocky Linux 9.3 (Blue Onyx)"},
		{"almalinux", alma98, "almalinux", "9.8", "AlmaLinux 9.8 (Olive Jaguar)"},
		{"sles", sles15sp5, "sles", "15.5", "SUSE Linux Enterprise Server 15 SP5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeFile(t, tt.content)
			useHostFile(t, path)

			got := ReadOSRelease()
			want := OSRelease{ID: tt.id, VersionID: tt.versionID, PrettyName: tt.prettyName, Source: OSSourceHostFile}
			if got != want {
				t.Fatalf("ReadOSRelease() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestParseOSReleaseSyntax(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    OSRelease
	}{
		{
			"comments and blank lines",
			"# a comment\n\n  \nID=debian\n# VERSION_ID=99\nVERSION_ID=12\n",
			OSRelease{ID: "debian", VersionID: "12"},
		},
		{
			"single quotes keep backslashes",
			"ID='debian'\nPRETTY_NAME='Debian \\\" 12'\n",
			OSRelease{ID: "debian", PrettyName: `Debian \" 12`},
		},
		{
			"double quote escapes",
			"ID=demo\nPRETTY_NAME=\"a \\\" b \\\\ c \\$ d \\` e\"\n",
			OSRelease{ID: "demo", PrettyName: "a \" b \\ c $ d ` e"},
		},
		{
			"unterminated quote kept verbatim",
			"ID=demo\nPRETTY_NAME=\"unbalanced\n",
			OSRelease{ID: "demo", PrettyName: `"unbalanced`},
		},
		{
			"line without separator",
			"ID=demo\ngarbage\n",
			OSRelease{ID: "demo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOSRelease(strings.NewReader(tt.content))
			if got != tt.want {
				t.Fatalf("parseOSRelease() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestReadOSReleaseFollowsRelativeSymlink(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "usr", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "etc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "usr", "lib", "os-release"), []byte(debian12), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "usr", "lib", "os-release"), filepath.Join(dir, "etc", "os-release")); err != nil {
		t.Fatal(err)
	}

	useHostFile(t, filepath.Join(dir, "etc", "os-release"))

	got := ReadOSRelease()
	if got.ID != "debian" || got.VersionID != "12" || got.Source != OSSourceHostFile {
		t.Fatalf("ReadOSRelease() = %+v", got)
	}
}

func TestReadOSReleaseInContainerWithoutMount(t *testing.T) {
	setPaths(t, filepath.Join(t.TempDir(), "absent"), writeFile(t, "ID=wrong\nVERSION_ID=1.0\nPRETTY_NAME=\"Container image\"\n"))
	t.Setenv("MAINTENANT_CONTAINER", "1")

	got := ReadOSRelease()
	want := OSRelease{UnavailableReason: OSReasonMountMissing}
	if got != want {
		t.Fatalf("ReadOSRelease() = %+v, want %+v", got, want)
	}
}

func TestInContainerTruthyValues(t *testing.T) {
	containerMarkers = nil
	t.Cleanup(func() { containerMarkers = []string{"/.dockerenv", "/run/.containerenv"} })

	for _, value := range []string{"1", "true", "TRUE", "Yes", "on", " on "} {
		t.Setenv("MAINTENANT_CONTAINER", value)
		if !inContainer() {
			t.Fatalf("inContainer() = false for %q", value)
		}
	}
	for _, value := range []string{"", "0", "false", "no", "maybe"} {
		t.Setenv("MAINTENANT_CONTAINER", value)
		if inContainer() {
			t.Fatalf("inContainer() = true for %q", value)
		}
	}
}

func TestInContainerMarkerFile(t *testing.T) {
	t.Setenv("MAINTENANT_CONTAINER", "")
	marker := writeFile(t, "")
	containerMarkers = []string{marker}
	t.Cleanup(func() { containerMarkers = []string{"/.dockerenv", "/run/.containerenv"} })

	if !inContainer() {
		t.Fatal("inContainer() = false with a marker file present")
	}
}

func TestReadOSReleaseUnreadable(t *testing.T) {
	t.Run("directory in place of the host file", func(t *testing.T) {
		dir := t.TempDir()
		mount := filepath.Join(dir, "os-release")
		if err := os.Mkdir(mount, 0o755); err != nil {
			t.Fatal(err)
		}
		useHostFile(t, mount)

		got := ReadOSRelease()
		want := OSRelease{UnavailableReason: OSReasonFileUnreadable}
		if got != want {
			t.Fatalf("ReadOSRelease() = %+v, want %+v", got, want)
		}
	})

	t.Run("no file outside a container", func(t *testing.T) {
		dir := t.TempDir()
		setPaths(t, filepath.Join(dir, "host-absent"), filepath.Join(dir, "absent"))
		t.Setenv("MAINTENANT_CONTAINER", "")

		got := ReadOSRelease()
		want := OSRelease{UnavailableReason: OSReasonFileUnreadable}
		if got != want {
			t.Fatalf("ReadOSRelease() = %+v, want %+v", got, want)
		}
	})

	t.Run("file without ID", func(t *testing.T) {
		useHostFile(t, writeFile(t, "NAME=\"Some system\"\nVERSION_ID=\"1\"\n"))

		got := ReadOSRelease()
		want := OSRelease{UnavailableReason: OSReasonFileUnreadable}
		if got != want {
			t.Fatalf("ReadOSRelease() = %+v, want %+v", got, want)
		}
	})
}

func TestParseOSImage(t *testing.T) {
	tests := []struct {
		osImage   string
		id        string
		versionID string
		reason    string
	}{
		{"Ubuntu 22.04.5 LTS", "ubuntu", "22.04", ""},
		{"Debian GNU/Linux 12 (bookworm)", "debian", "12", ""},
		{"Red Hat Enterprise Linux 9.4 (Plow)", "rhel", "9", ""},
		{"Rocky Linux 9.3 (Blue Onyx)", "rocky", "9", ""},
		{"AlmaLinux 9.8 (Olive Jaguar)", "almalinux", "9", ""},
		{"Alpine Linux v3.20", "alpine", "3.20", ""},
		{"SUSE Linux Enterprise Server 15 SP5", "sles", "15.5", ""},
		{"Flatcar Container Linux by Kinvolk 3815.2.0 (Oklo)", "", "", ""},
		{"Talos (v1.7.0)", "", "", ""},
		{"Bottlerocket OS 1.20.0 (aws-k8s-1.29)", "", "", ""},
		{"Linux", "", "", ""},
		{"Amazon Linux 2023.4.20240401", "", "", ""},
		{"", "", "", OSReasonNodeNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.osImage, func(t *testing.T) {
			got := ParseOSImage(tt.osImage)
			want := OSRelease{
				ID:                tt.id,
				VersionID:         tt.versionID,
				PrettyName:        tt.osImage,
				Source:            OSSourceKubernetesNode,
				UnavailableReason: tt.reason,
			}
			if tt.reason != "" {
				want.PrettyName = ""
			}
			if got != want {
				t.Fatalf("ParseOSImage(%q) = %+v, want %+v", tt.osImage, got, want)
			}
		})
	}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func useHostFile(t *testing.T, path string) {
	t.Helper()
	setPaths(t, path, filepath.Join(t.TempDir(), "unused"))
	t.Setenv("MAINTENANT_CONTAINER", "")
}

func setPaths(t *testing.T, hostPath, localPath string) {
	t.Helper()
	prevHost, prevLocal, prevMarkers := hostOSReleasePath, osReleasePath, containerMarkers
	hostOSReleasePath, osReleasePath, containerMarkers = hostPath, localPath, nil
	t.Cleanup(func() {
		hostOSReleasePath, osReleasePath, containerMarkers = prevHost, prevLocal, prevMarkers
	})
}

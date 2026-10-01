// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package app_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kolapsis/maintenant/internal/app"
)

var installScriptPath = filepath.Join("..", "..", "deploy", "install", "install.sh")

func readInstallScript(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile(installScriptPath)
	require.NoError(t, err)
	return string(src)
}

func scriptWordList(t *testing.T, src, name string) []string {
	t.Helper()
	start := strings.Index(src, "\n"+name+"=\"")
	require.NotEqual(t, -1, start, "%s is not assigned in install.sh", name)
	rest := src[start+len(name)+3:]
	end := strings.IndexByte(rest, '"')
	require.NotEqual(t, -1, end, "%s is not closed in install.sh", name)
	return strings.Fields(strings.ReplaceAll(rest[:end], "\\\n", " "))
}

func configurationFlags() (bools, values []string) {
	for _, spec := range app.Registry {
		switch {
		case spec.NoEnv:
		case spec.Type == app.FlagTypeBool:
			bools = append(bools, spec.FlagName)
		default:
			values = append(values, spec.FlagName)
		}
	}
	return bools, values
}

func TestInstallScript_AcceptsEveryConfigurationFlag(t *testing.T) {
	src := readInstallScript(t)
	bools, values := configurationFlags()

	assert.ElementsMatch(t, bools, scriptWordList(t, src, "BOOL_FLAGS"),
		"the boolean flags of install.sh must be the binary's FlagTypeBool flags")
	assert.ElementsMatch(t, values, scriptWordList(t, src, "VALUE_FLAGS"),
		"the value flags of install.sh must be the binary's other flags")
}

func TestInstallScript_HelpListsEveryConfigurationFlag(t *testing.T) {
	src := readInstallScript(t)
	start := strings.Index(src, "\nusage() {")
	require.NotEqual(t, -1, start)
	end := strings.Index(src[start:], "\nEOF\n")
	require.NotEqual(t, -1, end)
	help := src[start : start+end]

	bools, values := configurationFlags()
	for _, name := range append(bools, values...) {
		assert.Regexp(t, `(?m)^\s+--`+regexp.QuoteMeta(name)+`(\s|$)`, help, "install.sh --help omits --%s", name)
	}
}

func TestInstallScript_WritesTheEnvNamesTheBinaryReads(t *testing.T) {
	var names, want []string
	for _, spec := range app.Registry {
		if spec.NoEnv {
			continue
		}
		names = append(names, spec.FlagName)
		want = append(want, spec.EnvName)
	}

	args := append([]string{"-c", `. "$0"; for f in "$@"; do flag_to_env "$f"; echo; done`, installScriptPath}, names...)
	cmd := exec.Command("sh", args...)
	cmd.Env = append(os.Environ(), "_INSTALL_SH_TESTING=1", "NO_COLOR=1")
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, want, strings.Fields(string(out)))
}

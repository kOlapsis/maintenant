// Copyright 2026 Benjamin Touchard (kOlapsis)
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageKeepsTheDatabaseOnTheDataVolume(t *testing.T) {
	dockerfile, err := os.ReadFile(filepath.Join("..", "..", "Dockerfile"))
	require.NoError(t, err)

	env := map[string]string{}
	var volumes []string
	for _, line := range strings.Split(string(dockerfile), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "ENV":
			for _, kv := range fields[1:] {
				if k, v, ok := strings.Cut(kv, "="); ok {
					env[k] = v
				}
			}
		case "VOLUME":
			volumes = append(volumes, fields[1:]...)
		}
	}

	db := env["MAINTENANT_DB"]
	require.NotEmpty(t, db, "without MAINTENANT_DB the binary opens ./maintenant.db on the read-only root")
	assert.Contains(t, volumes, filepath.Dir(db))
}

type entrypointRun struct {
	stubs string
	log   string
}

// newEntrypointRun stubs every command the entrypoint runs, so it can be driven as root without touching the host.
func newEntrypointRun(t *testing.T) *entrypointRun {
	t.Helper()
	stubs := t.TempDir()
	record := "#!/bin/sh\nprintf '%s %s\\n' \"${0##*/}\" \"$*\" >>\"$STUB_LOG\"\n"
	for _, name := range []string{"chown", "mkdir", "setpriv", "stat", "maintenant"} {
		require.NoError(t, os.WriteFile(filepath.Join(stubs, name), []byte(record), 0o700))
	}
	id := "#!/bin/sh\necho \"$STUB_UID\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(stubs, "id"), []byte(id), 0o700))
	return &entrypointRun{stubs: stubs, log: filepath.Join(stubs, "calls.log")}
}

func (e *entrypointRun) binary() string { return filepath.Join(e.stubs, "maintenant") }

// run executes the entrypoint as uid and returns the stub calls, one "name args" per entry.
func (e *entrypointRun) run(t *testing.T, uid string, env map[string]string, args ...string) []string {
	t.Helper()
	cmd := exec.Command("/bin/sh", append([]string{filepath.Join("..", "..", "docker-entrypoint.sh")}, args...)...)
	cmd.Env = []string{"PATH=" + e.stubs + ":" + os.Getenv("PATH"), "STUB_LOG=" + e.log, "STUB_UID=" + uid}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	data, err := os.ReadFile(e.log)
	require.NoError(t, err)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func calls(all []string, name string) []string {
	var out []string
	for _, c := range all {
		if cmd, args, _ := strings.Cut(c, " "); cmd == name {
			out = append(out, args)
		}
	}
	return out
}

func chowned(all []string) []string {
	var dirs []string
	for _, args := range calls(all, "chown") {
		f := strings.Fields(args)
		dirs = append(dirs, f[len(f)-1])
	}
	return dirs
}

func TestEntrypoint_UnprivilegedRunsTheCommandItself(t *testing.T) {
	e := newEntrypointRun(t)
	got := e.run(t, "65534", nil, e.binary(), "--mode=agent")

	assert.Equal(t, []string{"maintenant --mode=agent"}, got)
}

func TestEntrypoint_AgentOwnsItsDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	cases := map[string]struct {
		env  map[string]string
		args []string
	}{
		"mode and data dir from the environment": {
			env: map[string]string{"MAINTENANT_MODE": "agent", "MAINTENANT_DATA_DIR": dataDir},
		},
		"mode and data dir from flags": {
			args: []string{"--mode", "agent", "--data-dir=" + dataDir},
		},
		"mode flag, data dir from the environment": {
			env:  map[string]string{"MAINTENANT_DATA_DIR": dataDir},
			args: []string{"-mode=agent"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEntrypointRun(t)
			got := e.run(t, "0", tc.env, append([]string{e.binary()}, tc.args...)...)

			assert.Equal(t, []string{dataDir}, chowned(got))
			assert.Len(t, calls(got, "setpriv"), 1)
		})
	}
}

func TestEntrypoint_ServerOwnsTheDatabaseDirectory(t *testing.T) {
	dbDir := t.TempDir()
	flagDir := t.TempDir()

	e := newEntrypointRun(t)
	got := e.run(t, "0", map[string]string{"MAINTENANT_DB": filepath.Join(dbDir, "maintenant.db")}, e.binary())
	assert.ElementsMatch(t, []string{dbDir, "/data/shm"}, chowned(got))
	assert.Len(t, calls(got, "setpriv"), 1)

	e = newEntrypointRun(t)
	got = e.run(t, "0", map[string]string{"MAINTENANT_MODE": "agent"}, e.binary(), "--mode=server", "--db", filepath.Join(flagDir, "m.db"))
	assert.ElementsMatch(t, []string{flagDir, "/data/shm"}, chowned(got))
}

func TestEntrypoint_NeverHandsTheRootDirectoryOver(t *testing.T) {
	e := newEntrypointRun(t)
	got := e.run(t, "0", map[string]string{"MAINTENANT_DB": "/maintenant.db"}, e.binary())

	assert.Equal(t, []string{"/data/shm"}, chowned(got))
}

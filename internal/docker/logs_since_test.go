// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Acceptance tests of 047-audit-small-fixes, C13: the exit snippet is bounded to the last start.

type logsAPI struct {
	client.APIClient
	opts client.ContainerLogsOptions
	body []byte
}

func (f *logsAPI) ContainerLogs(_ context.Context, _ string, opts client.ContainerLogsOptions) (client.ContainerLogsResult, error) {
	f.opts = opts
	return io.NopCloser(bytes.NewReader(f.body)), nil
}

// stdoutFrames multiplexes lines the way the Docker daemon does for a non-TTY container: an 8-byte header (stream, 3 zero bytes, big-endian length) before each payload.
func stdoutFrames(t *testing.T, lines ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	for _, l := range lines {
		payload := []byte(l + "\n")
		header := []byte{byte(stdcopy.Stdout), 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(header[4:], uint32(len(payload)))
		buf.Write(header)
		buf.Write(payload)
	}
	return buf.Bytes()
}

// parseDockerSince reads the Since value in either form the Docker API accepts.
func parseDockerSince(t *testing.T, raw string) time.Time {
	t.Helper()
	if ts, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return ts
	}
	sec, frac, _ := strings.Cut(raw, ".")
	s, err := strconv.ParseInt(sec, 10, 64)
	require.NoError(t, err, "Since %q is neither RFC 3339 nor unix seconds", raw)
	var ns int64
	if frac != "" {
		frac = (frac + "000000000")[:9]
		ns, err = strconv.ParseInt(frac, 10, 64)
		require.NoError(t, err)
	}
	return time.Unix(s, ns)
}

// FR-011, contract "Extrait de journal": the daemon is asked for the lines after the start time, 50 at most.
func TestFetchLogSnippetSince_AsksForTheLinesAfterTheStart(t *testing.T) {
	for _, since := range []time.Time{
		time.Date(2026, 10, 21, 8, 30, 15, 0, time.UTC),
		time.Now().Add(-90 * time.Second).Truncate(time.Second),
	} {
		api := &logsAPI{body: stdoutFrames(t, "boot ok", "listening on :8080", "panic: out of memory")}
		c := &Client{cli: api, logger: slog.New(slog.NewTextHandler(io.Discard, nil))}

		snippet, err := c.FetchLogSnippetSince(context.Background(), "deadbeef", since)
		require.NoError(t, err)

		assert.Equal(t, "boot ok\nlistening on :8080\npanic: out of memory", snippet)
		assert.True(t, api.opts.ShowStdout && api.opts.ShowStderr)
		assert.Equal(t, snippetTail, api.opts.Tail, "50 lines at most, as today")
		require.NotEmpty(t, api.opts.Since, "the request must carry since")
		assert.True(t, parseDockerSince(t, api.opts.Since).Equal(since), "Since %q is not %s", api.opts.Since, since)
	}
}

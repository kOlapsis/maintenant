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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadMeminfo(t *testing.T) {
	dir := t.TempDir()
	content := "MemTotal:       16384000 kB\nMemFree:         1000000 kB\nMemAvailable:    8192000 kB\nBuffers:          200000 kB\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "meminfo"), []byte(content), 0o600))

	total, avail, err := readMeminfo(dir)
	require.NoError(t, err)
	assert.Equal(t, int64(16384000)*1024, total)
	assert.Equal(t, int64(8192000)*1024, avail)
}

func TestReadMeminfo_MissingFile(t *testing.T) {
	_, _, err := readMeminfo(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.Error(t, err)
}

func TestReadCPUJiffies(t *testing.T) {
	dir := t.TempDir()
	// user=100 nice=0 system=50 idle=800 iowait=20 irq=0 softirq=10 steal=0
	content := "cpu  100 0 50 800 20 0 10 0 0 0\ncpu0 50 0 25 400 10 0 5 0 0 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stat"), []byte(content), 0o600))

	j, err := readCPUJiffies(dir)
	require.NoError(t, err)
	// active = user+nice+system+irq+softirq+steal = 160
	assert.Equal(t, uint64(160), j.active)
	// total = active + idle + iowait = 160 + 800 + 20 = 980
	assert.Equal(t, uint64(980), j.total)
}

func TestReadCPUJiffies_MissingFile(t *testing.T) {
	_, err := readCPUJiffies(filepath.Join(t.TempDir(), "does-not-exist"))
	assert.Error(t, err)
}

func TestDiskUsage_Root(t *testing.T) {
	total, used := DiskUsage("/")
	assert.Positive(t, total, "root filesystem must report a total size")
	assert.LessOrEqual(t, used, total, "used must never exceed total")
}

func TestDiskUsage_BadPath(t *testing.T) {
	total, used := DiskUsage("/no/such/path/should/exist/xyz")
	assert.Zero(t, total)
	assert.Zero(t, used)
}

func TestReader_SampleReadsLocalProc(t *testing.T) {
	r := NewReader()
	// NewReader only primes the CPU counter; sample() fills memory.
	r.sample()
	assert.Positive(t, r.MemTotal(), "MemTotal should be read from /proc/meminfo")
	assert.GreaterOrEqual(t, r.MemUsed(), int64(0))
	assert.GreaterOrEqual(t, r.CPUPercent(), 0.0)
}

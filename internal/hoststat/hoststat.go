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

// Package hoststat reads host-level CPU, memory and disk usage from /proc and
// the filesystem. It has no dependencies on the rest of the codebase so both
// the server's resource service and the remote agent collector can use it.
package hoststat

import (
	"bufio"
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// hostProcPath is where the host /proc is mounted inside the container.
	// Deploy with: -v /proc:/host/proc:ro
	// Falls back to /proc when running outside a container.
	hostProcPath     = "/host/proc"
	fallbackProcPath = "/proc"

	// sampleInterval matches htop's default refresh rate for comparable values.
	sampleInterval = 1 * time.Second
)

// Reader samples host CPU and memory at 1s intervals via /proc. It runs its
// own goroutine, independent of any container collector.
type Reader struct {
	mu         sync.RWMutex
	procPath   string
	prev       cpuJiffies
	cpuPercent float64
	memUsed    int64
	memTotal   int64
}

// cpuJiffies holds the cumulative CPU time counters from /proc/stat.
type cpuJiffies struct {
	active uint64 // user + nice + system + irq + softirq + steal
	total  uint64 // active + idle + iowait
}

// NewReader creates a reader and takes an initial CPU sample.
func NewReader() *Reader {
	procPath := hostProcPath
	if _, err := os.Stat(procPath + "/stat"); err != nil {
		procPath = fallbackProcPath
	}

	r := &Reader{procPath: procPath}
	if j, err := readCPUJiffies(procPath); err == nil {
		r.prev = j
	}
	return r
}

// Start runs the sampling goroutine. Blocks until ctx is cancelled.
func (r *Reader) Start(ctx context.Context) {
	ticker := time.NewTicker(sampleInterval)
	defer ticker.Stop()

	// Take an initial reading immediately.
	r.sample()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.sample()
		}
	}
}

func (r *Reader) sample() {
	// CPU: delta of jiffies since last sample (same method as htop).
	if cur, err := readCPUJiffies(r.procPath); err == nil {
		r.mu.Lock()
		deltaTotal := cur.total - r.prev.total
		deltaActive := cur.active - r.prev.active
		if deltaTotal > 0 {
			r.cpuPercent = float64(deltaActive) / float64(deltaTotal) * 100.0
		}
		r.prev = cur
		r.mu.Unlock()
	}

	// Memory: instant reading from /proc/meminfo.
	if total, avail, err := readMeminfo(r.procPath); err == nil {
		r.mu.Lock()
		r.memTotal = total
		r.memUsed = total - avail
		r.mu.Unlock()
	}
}

// CPUPercent returns the latest host CPU usage percentage (0-100).
func (r *Reader) CPUPercent() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cpuPercent
}

// MemUsed returns host memory used in bytes.
func (r *Reader) MemUsed() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.memUsed
}

// MemTotal returns host total memory in bytes.
func (r *Reader) MemTotal() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.memTotal
}

// DiskUsage returns total and used bytes of the filesystem holding path.
// Returns (0, 0) if the filesystem cannot be stat'd.
func DiskUsage(path string) (total uint64, used uint64) {
	var fs syscall.Statfs_t
	if err := syscall.Statfs(path, &fs); err != nil {
		return 0, 0
	}
	total = fs.Blocks * uint64(fs.Bsize)
	free := fs.Bavail * uint64(fs.Bsize)
	if total >= free {
		used = total - free
	}
	return total, used
}

// readCPUJiffies parses the aggregate "cpu" line of /proc/stat.
//
// Format: cpu user nice system idle iowait irq softirq steal [guest guest_nice]
//
// htop-compatible: active = user + nice + system + irq + softirq + steal
// (guest/guest_nice are already included in user/nice per kernel docs).
// idle = idle + iowait (iowait is time the CPU was available but waiting).
func readCPUJiffies(procPath string) (cpuJiffies, error) {
	f, err := os.Open(procPath + "/stat")
	if err != nil {
		return cpuJiffies{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			break
		}

		// Parse fields 1..8: user nice system idle iowait irq softirq steal
		var vals [8]uint64
		for i := 0; i < 8 && i+1 < len(fields); i++ {
			vals[i], _ = strconv.ParseUint(fields[i+1], 10, 64)
		}

		user, nice, system := vals[0], vals[1], vals[2]
		idle, iowait := vals[3], vals[4]
		irq, softirq, steal := vals[5], vals[6], vals[7]

		active := user + nice + system + irq + softirq + steal
		total := active + idle + iowait

		return cpuJiffies{active: active, total: total}, nil
	}

	return cpuJiffies{}, os.ErrNotExist
}

// readMeminfo reads MemTotal and MemAvailable from /proc/meminfo (bytes).
func readMeminfo(procPath string) (total int64, available int64, err error) {
	f, err := os.Open(procPath + "/meminfo")
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()

	found := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && found < 2 {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			total = parseMeminfoKB(line) * 1024
			found++
		} else if strings.HasPrefix(line, "MemAvailable:") {
			available = parseMeminfoKB(line) * 1024
			found++
		}
	}
	return total, available, nil
}

func parseMeminfoKB(line string) int64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseInt(fields[1], 10, 64)
	return v
}

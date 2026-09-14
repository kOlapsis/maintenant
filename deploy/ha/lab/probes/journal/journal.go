// Package journal reads back what a probe already wrote.
//
// Both probes append to their journal and are restarted by systemd if they
// die. A restart must not renumber the records: the verification keys on the
// sequence number, and two records sharing one would be read as a single
// write. So a probe picks up where the journal left off.
package journal

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

// maxTail is how far back the last complete line is looked for.
const maxTail = 64 * 1024

type seqOnly struct {
	Seq uint64 `json:"seq"`
}

// LastSeq returns the highest sequence number the journal at path ends with,
// or zero when the file is absent or holds no usable record.
func LastSeq(path string) (uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}

	size := info.Size()
	start := size - maxTail
	if start < 0 {
		start = 0
	}
	buf := make([]byte, size-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return 0, err
	}

	// The last line can be a partial write left by a kill, so walk backwards
	// until one parses.
	end := len(buf)
	for end > 0 {
		if buf[end-1] == '\n' {
			end--
			continue
		}
		begin := end
		for begin > 0 && buf[begin-1] != '\n' {
			begin--
		}
		var s seqOnly
		if json.Unmarshal(buf[begin:end], &s) == nil && s.Seq > 0 {
			return s.Seq, nil
		}
		end = begin
	}
	return 0, nil
}

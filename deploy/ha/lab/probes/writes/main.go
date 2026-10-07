// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: LicenseRef-Maintenant-Commercial
// See deploy/ha/LICENSE.

// Command writes measures data loss across a failover.
//
// It writes numbered records through the product's API and journals what the
// server answered for each one, in three classes: acknowledged, refused, and
// unknown when no answer came back at all. After the service is back it reads
// every record again and compares. Only acknowledged records that are missing
// count as loss; an unknown is never counted as a loss and never as a success
// either, because nobody can say whether it committed (FR-016, FR-019).
//
// The record is a heartbeat ping: the pings table is append-only, uncapped, and
// is the busiest write path the product has, so the instrument writes the way
// the product does. The server answers the id of the ping it stored, which the
// journal keeps and the read-back looks for.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/kolapsis/maintenant/deploy/ha/lab/probes/journal"
)

type record struct {
	Seq    uint64 `json:"seq"`
	Wall   string `json:"wall"`
	Epoch  string `json:"epoch"`
	MonoNS int64  `json:"mono_ns"`
	Class  string `json:"class"`
	Status int    `json:"status,omitempty"`
	ID     string `json:"id,omitempty"`
	Error  string `json:"error,omitempty"`
}

const (
	classAck     = "ack"
	classRefused = "refused"
	classUnknown = "unknown"
)

func main() {
	mode := flag.String("mode", "generate", "generate or verify")
	base := flag.String("base", "", "base URL of the service address")
	token := flag.String("token", "", "ping token of the heartbeat written to")
	id := flag.String("heartbeat", "", "heartbeat id, to read the records back")
	rate := flag.Duration("interval", 100*time.Millisecond, "delay between two writes")
	timeout := flag.Duration("timeout", 2*time.Second, "per-request timeout")
	journal := flag.String("journal", "writes.ndjson", "ndjson journal of what the server answered")
	from := flag.String("from", "", "verify: only judge records written at or after this instant, RFC3339")
	to := flag.String("to", "", "verify: only judge records written at or before this instant, RFC3339")
	flag.Parse()

	if *base == "" {
		fail("-base is required")
	}

	var err error
	switch *mode {
	case "generate":
		if *token == "" {
			fail("-token is required to generate")
		}
		err = generate(*base, *token, *rate, *timeout, *journal)
	case "verify":
		if *id == "" {
			fail("-heartbeat is required to verify")
		}
		var w window
		if w, err = parseWindow(*from, *to); err != nil {
			fail(err.Error())
		}
		err = verify(*base, *id, *timeout, *journal, w)
	default:
		fail("-mode must be generate or verify")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "writes: %v\n", err)
		os.Exit(1)
	}
}

func fail(msg string) {
	fmt.Fprintf(os.Stderr, "writes: %s\n", msg)
	os.Exit(2)
}

func client(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: true,
			DialContext:       (&net.Dialer{Timeout: timeout}).DialContext,
		},
	}
}

func generate(base, token string, interval, timeout time.Duration, journalPath string) error {
	resume, err := journal.LastSeq(journalPath)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(journalPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	encoder := json.NewEncoder(f)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := client(timeout)
	url := base + "/ping/" + token
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	start := time.Now()
	epoch := start.UTC().Format(time.RFC3339Nano)
	seq := resume

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			seq++
			// Writes are serial on purpose: a record is only ever sent after
			// the previous one was answered or gave up, so the journal reads
			// as the order the service saw.
			r := write(c, url, seq, start, epoch, timeout)
			if err := encoder.Encode(r); err != nil {
				return err
			}
		}
	}
}

// The request deliberately does not hang off the shutdown context: a write
// already on the wire when the probe is asked to stop must be allowed to
// finish, or every run would end with a spurious unknown in its tally.
func write(c *http.Client, url string, seq uint64, start time.Time, epoch string, timeout time.Duration) record {
	at := time.Now()
	r := record{
		Seq:    seq,
		Wall:   at.UTC().Format(time.RFC3339Nano),
		Epoch:  epoch,
		MonoNS: at.Sub(start).Nanoseconds(),
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, nil)
	if err != nil {
		r.Class = classRefused
		r.Error = err.Error()
		return r
	}

	resp, err := c.Do(req)
	if err != nil {
		r.Class = classifyWriteError(err)
		r.Error = err.Error()
		return r
	}
	defer func() { _ = resp.Body.Close() }()

	r.Status = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		r.Class = classRefused
		return r
	}
	r.Class = classAck
	var answer struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&answer); err != nil {
		r.Error = "acknowledged, answer unreadable: " + err.Error()
	}
	r.ID = answer.ID
	return r
}

// A write that never left the machine did not happen, and is refused. A write
// that left and got no answer may or may not have committed, and is unknown:
// counting it either way would be a guess, and the RPO is not a guess.
func classifyWriteError(err error) string {
	var dnsErr *net.DNSError
	switch {
	case errors.Is(err, syscall.ECONNREFUSED),
		errors.Is(err, syscall.EHOSTUNREACH),
		errors.Is(err, syscall.ENETUNREACH),
		errors.As(err, &dnsErr):
		return classRefused
	default:
		return classUnknown
	}
}

type window struct {
	from, to time.Time
}

func parseWindow(fromRaw, toRaw string) (window, error) {
	var w window
	var err error
	if fromRaw != "" {
		if w.from, err = time.Parse(time.RFC3339, fromRaw); err != nil {
			return w, fmt.Errorf("-from: %w", err)
		}
	}
	if toRaw != "" {
		if w.to, err = time.Parse(time.RFC3339, toRaw); err != nil {
			return w, fmt.Errorf("-to: %w", err)
		}
	}
	return w, nil
}

func (w window) holds(wall string) bool {
	if w.from.IsZero() && w.to.IsZero() {
		return true
	}
	at, err := time.Parse(time.RFC3339Nano, wall)
	if err != nil {
		return false
	}
	return (w.from.IsZero() || !at.Before(w.from)) && (w.to.IsZero() || !at.After(w.to))
}

// holdsStored widens the window by a second on each side: stored timestamps are truncated to the second.
func (w window) holdsStored(at time.Time) bool {
	return (w.from.IsZero() || !at.Before(w.from.Add(-time.Second))) && (w.to.IsZero() || !at.After(w.to.Add(time.Second)))
}

func (w window) bound(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

type tally struct {
	From           string   `json:"from,omitempty"`
	To             string   `json:"to,omitempty"`
	Ack            int      `json:"ack"`
	Refused        int      `json:"refused"`
	Unknown        int      `json:"unknown"`
	Lost           int      `json:"lost"`
	LostSeqs       []uint64 `json:"lost_seqs"`
	Unrecorded     int      `json:"unrecorded"`
	UnknownPresent int      `json:"unknown_present"`
	UnknownAbsent  int      `json:"unknown_absent"`
	Recorded       int      `json:"recorded"`
}

func verify(base, id string, timeout time.Duration, journalPath string, w window) error {
	f, err := os.Open(journalPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	t := tally{From: w.bound(w.from), To: w.bound(w.to), LostSeqs: []uint64{}}
	acked := map[uint64]string{}
	claimed := map[string]bool{}

	decoder := json.NewDecoder(f)
	for {
		var r record
		if err := decoder.Decode(&r); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("journal %s: %w", journalPath, err)
		}
		if r.Class == classAck && r.ID != "" {
			claimed[r.ID] = true
		}
		if !w.holds(r.Wall) {
			continue
		}
		switch r.Class {
		case classAck:
			t.Ack++
			acked[r.Seq] = r.ID
		case classRefused:
			t.Refused++
		case classUnknown:
			t.Unknown++
		}
	}

	stored, err := readBack(base, id, timeout)
	if err != nil {
		return err
	}
	t.Recorded = len(stored)

	// An acknowledgement without an id is the server saying it answered 200 without storing the ping.
	for seq, pingID := range acked {
		if pingID == "" {
			t.Unrecorded++
		}
		if _, ok := stored[pingID]; !ok {
			t.Lost++
			t.LostSeqs = append(t.LostSeqs, seq)
		}
	}
	sort.Slice(t.LostSeqs, func(i, j int) bool { return t.LostSeqs[i] < t.LostSeqs[j] })

	// Only this probe writes to the heartbeat, so a stored ping no acknowledgement claims is an unknown that committed.
	for pingID, at := range stored {
		if !claimed[pingID] && w.holdsStored(at) {
			t.UnknownPresent++
		}
	}
	t.UnknownAbsent = max(t.Unknown-t.UnknownPresent, 0)

	out, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(out))
	if t.Lost > 0 {
		return fmt.Errorf("%d acknowledged writes are missing", t.Lost)
	}
	return nil
}

func readBack(base, id string, timeout time.Duration) (map[string]time.Time, error) {
	c := client(timeout)
	stored := map[string]time.Time{}
	const page = 500
	// The API orders pings by a timestamp in whole seconds, so rows sharing one may swap between two pages: pages overlap.
	const stride = page - 100

	for offset := 0; ; offset += stride {
		url := fmt.Sprintf("%s/api/v1/heartbeats/%s/pings?limit=%d&offset=%d", base, id, page, offset)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			cancel()
			return nil, err
		}
		resp, err := c.Do(req)
		if err != nil {
			cancel()
			return nil, err
		}
		var body struct {
			Pings []struct {
				ID        string    `json:"id"`
				Timestamp time.Time `json:"timestamp"`
			} `json:"pings"`
			Total int `json:"total"`
		}
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		cancel()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("reading records back: %s", resp.Status)
		}
		for _, p := range body.Pings {
			stored[p.ID] = p.Timestamp
		}
		if len(body.Pings) < page || offset+len(body.Pings) >= body.Total {
			return stored, nil
		}
	}
}

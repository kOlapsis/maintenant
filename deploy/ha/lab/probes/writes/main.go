// Command writes measures data loss across a failover.
//
// It writes numbered records through the product's API and journals what the
// server answered for each one, in three classes: acknowledged, refused, and
// unknown when no answer came back at all. After the service is back it reads
// every record again and compares. Only acknowledged records that are missing
// count as loss; an unknown is never counted as a loss and never as a success
// either, because nobody can say whether it committed (FR-016, FR-019).
//
// The record is a heartbeat ping carrying a sequence number as its payload:
// the pings table is append-only, uncapped, and is the busiest write path the
// product has, so the instrument writes the way the product does.
package main

import (
	"bytes"
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
	Error  string `json:"error,omitempty"`
}

type payload struct {
	Seq uint64 `json:"seq"`
	At  string `json:"at"`
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
		err = verify(*base, *id, *timeout, *journal)
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

	body, err := json.Marshal(payload{Seq: seq, At: r.Wall})
	if err != nil {
		r.Class = classRefused
		r.Error = err.Error()
		return r
	}

	reqCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		r.Class = classRefused
		r.Error = err.Error()
		return r
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		r.Class = classifyWriteError(err)
		r.Error = err.Error()
		return r
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	r.Status = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		r.Class = classAck
	} else {
		r.Class = classRefused
	}
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

type tally struct {
	Ack            int      `json:"ack"`
	Refused        int      `json:"refused"`
	Unknown        int      `json:"unknown"`
	Lost           int      `json:"lost"`
	LostSeqs       []uint64 `json:"lost_seqs"`
	UnknownPresent int      `json:"unknown_present"`
	UnknownAbsent  int      `json:"unknown_absent"`
	Recorded       int      `json:"recorded"`
}

func verify(base, id string, timeout time.Duration, journalPath string) error {
	f, err := os.Open(journalPath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	t := tally{LostSeqs: []uint64{}}
	acked := map[uint64]bool{}
	unknown := map[uint64]bool{}

	decoder := json.NewDecoder(f)
	for {
		var r record
		if err := decoder.Decode(&r); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return fmt.Errorf("journal %s: %w", journalPath, err)
		}
		switch r.Class {
		case classAck:
			t.Ack++
			acked[r.Seq] = true
		case classRefused:
			t.Refused++
		case classUnknown:
			t.Unknown++
			unknown[r.Seq] = true
		}
	}

	present, err := readBack(base, id, timeout)
	if err != nil {
		return err
	}
	t.Recorded = len(present)

	for seq := range acked {
		if !present[seq] {
			t.Lost++
			t.LostSeqs = append(t.LostSeqs, seq)
		}
	}
	for seq := range unknown {
		if present[seq] {
			t.UnknownPresent++
		} else {
			t.UnknownAbsent++
		}
	}

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

func readBack(base, id string, timeout time.Duration) (map[uint64]bool, error) {
	c := client(timeout)
	present := map[uint64]bool{}
	const page = 500

	for offset := 0; ; offset += page {
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
				Payload *string `json:"payload"`
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
			if p.Payload == nil {
				continue
			}
			var pl payload
			if json.Unmarshal([]byte(*p.Payload), &pl) == nil && pl.Seq > 0 {
				present[pl.Seq] = true
			}
		}
		if len(body.Pings) < page || offset+len(body.Pings) >= body.Total {
			return present, nil
		}
	}
}

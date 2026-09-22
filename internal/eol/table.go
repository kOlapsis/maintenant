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

// Package eol resolves the end-of-support dates of host operating systems.
package eol

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

const (
	SourceEmbedded = "embedded"
	SourceRemote   = "endoflife.date"
)

// Products lists the endoflife.date slugs the table covers.
var Products = []string{"debian", "ubuntu", "rhel", "rocky-linux", "almalinux", "alpine-linux", "sles"}

//go:embed table.json
var embeddedTable []byte

// Date is a calendar day, without time or zone.
type Date struct {
	t time.Time
}

// NewDate builds a Date from its calendar components.
func NewDate(year int, month time.Month, day int) Date {
	return Date{t: time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate reads a YYYY-MM-DD day.
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation(time.DateOnly, s, time.UTC)
	if err != nil {
		return Date{}, fmt.Errorf("parse date %q: %w", s, err)
	}
	return Date{t: t}, nil
}

func (d Date) String() string { return d.t.Format(time.DateOnly) }

func (d Date) IsZero() bool { return d.t.IsZero() }

// After reports whether d is a later day than other.
func (d Date) After(other Date) bool { return d.t.After(other.t) }

// DaysUntil returns the whole days from today to d, negative once d has passed.
func (d Date) DaysUntil(today time.Time) int {
	return int(d.t.Sub(truncateDay(today)).Hours() / 24)
}

func (d Date) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Date) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("decode date: %w", err)
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

func truncateDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// Cycle is one release line of a product, with the dates its support ends.
type Cycle struct {
	Name          string `json:"name"`
	LTS           bool   `json:"lts"`
	ActiveUntil   *Date  `json:"active_until,omitempty"`
	SecurityUntil Date   `json:"security_until"`
	ExtendedUntil *Date  `json:"extended_until,omitempty"`
}

// Table holds the support cycles of every tracked product.
type Table struct {
	Source    string             `json:"source"`
	FetchedAt time.Time          `json:"fetched_at"`
	Products  map[string][]Cycle `json:"products"`
}

// Cycle returns the cycle of a product by name.
func (t Table) Cycle(product, name string) (Cycle, bool) {
	for _, c := range t.Products[product] {
		if c.Name == name {
			return c, true
		}
	}
	return Cycle{}, false
}

// LoadEmbedded reads the table shipped with the binary.
func LoadEmbedded() (Table, error) {
	var t Table
	if err := json.Unmarshal(embeddedTable, &t); err != nil {
		return Table{}, fmt.Errorf("decode embedded table: %w", err)
	}
	if err := Validate(t); err != nil {
		return Table{}, fmt.Errorf("embedded table: %w", err)
	}
	return t, nil
}

// Validate checks that a table covers every product and that its dates are ordered.
func Validate(t Table) error {
	for _, product := range Products {
		cycles := t.Products[product]
		if len(cycles) == 0 {
			return fmt.Errorf("product %s: no cycle", product)
		}
		seen := make(map[string]struct{}, len(cycles))
		for _, c := range cycles {
			if c.Name == "" {
				return fmt.Errorf("product %s: cycle without name", product)
			}
			if _, dup := seen[c.Name]; dup {
				return fmt.Errorf("product %s: duplicate cycle %s", product, c.Name)
			}
			seen[c.Name] = struct{}{}
			if c.SecurityUntil.IsZero() {
				return fmt.Errorf("product %s cycle %s: no security_until", product, c.Name)
			}
			if c.ActiveUntil != nil && c.ActiveUntil.After(c.SecurityUntil) {
				return fmt.Errorf("product %s cycle %s: active_until %s after security_until %s", product, c.Name, c.ActiveUntil, c.SecurityUntil)
			}
			if c.ExtendedUntil != nil && c.SecurityUntil.After(*c.ExtendedUntil) {
				return fmt.Errorf("product %s cycle %s: security_until %s after extended_until %s", product, c.Name, c.SecurityUntil, c.ExtendedUntil)
			}
		}
	}
	return nil
}

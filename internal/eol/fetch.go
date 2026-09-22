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

package eol

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	defaultBaseURL   = "https://endoflife.date"
	defaultUserAgent = "maintenant/1.0 (+https://maintenant.dev)"
	fetchTimeout     = 15 * time.Second
)

// Fetcher downloads the support cycles from the endoflife.date API.
type Fetcher struct {
	BaseURL   string
	Client    *http.Client
	UserAgent string
	Now       func() time.Time
}

type apiResponse struct {
	Result struct {
		Releases []apiRelease `json:"releases"`
	} `json:"result"`
}

type apiRelease struct {
	Name     string  `json:"name"`
	IsLTS    bool    `json:"isLts"`
	EoasFrom *string `json:"eoasFrom"`
	EolFrom  *string `json:"eolFrom"`
	EoesFrom *string `json:"eoesFrom"`
}

// Fetch downloads every tracked product; a single failure yields no table at all.
func (f *Fetcher) Fetch(ctx context.Context) (Table, error) {
	table := Table{
		Source:    SourceRemote,
		FetchedAt: f.now(),
		Products:  make(map[string][]Cycle, len(Products)),
	}
	for _, product := range Products {
		cycles, err := f.fetchProduct(ctx, product)
		if err != nil {
			return Table{}, err
		}
		table.Products[product] = cycles
	}
	if err := Validate(table); err != nil {
		return Table{}, fmt.Errorf("fetched table: %w", err)
	}
	return table, nil
}

func (f *Fetcher) fetchProduct(ctx context.Context, product string) ([]Cycle, error) {
	url := fmt.Sprintf("%s/api/v1/products/%s", f.baseURL(), product)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", product, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", f.userAgent())

	resp, err := f.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("product %s: %w", product, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("product %s: endoflife.date returned status %d", product, resp.StatusCode)
	}

	var payload apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("product %s: decode response: %w", product, err)
	}
	if len(payload.Result.Releases) == 0 {
		return nil, fmt.Errorf("product %s: no release", product)
	}

	cycles := make([]Cycle, 0, len(payload.Result.Releases))
	for _, release := range payload.Result.Releases {
		cycle, err := toCycle(release)
		if err != nil {
			return nil, fmt.Errorf("product %s: %w", product, err)
		}
		cycles = append(cycles, cycle)
	}
	return cycles, nil
}

func toCycle(release apiRelease) (Cycle, error) {
	if release.EolFrom == nil {
		return Cycle{}, fmt.Errorf("release %s: no eolFrom", release.Name)
	}
	securityUntil, err := ParseDate(*release.EolFrom)
	if err != nil {
		return Cycle{}, fmt.Errorf("release %s: eolFrom: %w", release.Name, err)
	}
	cycle := Cycle{Name: release.Name, LTS: release.IsLTS, SecurityUntil: securityUntil}
	if release.EoasFrom != nil {
		activeUntil, err := ParseDate(*release.EoasFrom)
		if err != nil {
			return Cycle{}, fmt.Errorf("release %s: eoasFrom: %w", release.Name, err)
		}
		cycle.ActiveUntil = &activeUntil
	}
	if release.EoesFrom != nil {
		extendedUntil, err := ParseDate(*release.EoesFrom)
		if err != nil {
			return Cycle{}, fmt.Errorf("release %s: eoesFrom: %w", release.Name, err)
		}
		cycle.ExtendedUntil = &extendedUntil
	}
	return cycle, nil
}

func (f *Fetcher) baseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}
	return defaultBaseURL
}

func (f *Fetcher) userAgent() string {
	if f.UserAgent != "" {
		return f.UserAgent
	}
	return defaultUserAgent
}

func (f *Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{Timeout: fetchTimeout}
}

func (f *Fetcher) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

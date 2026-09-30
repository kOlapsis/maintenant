// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package endpoint

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kolapsis/maintenant/internal/container"
)

const (
	labelPrefix = "maintenant.endpoint."
)

// MinInterval is the shortest check interval an endpoint may have.
var MinInterval = 5 * time.Second

// Indexed label regex: maintenant.endpoint.N.http or maintenant.endpoint.N.tcp
var indexedLabelRe = regexp.MustCompile(`^maintenant\.endpoint\.(\d+)\.(http|tcp)$`)

// Indexed config regex: maintenant.endpoint.N.http.method, maintenant.endpoint.N.interval, etc.
var indexedConfigRe = regexp.MustCompile(`^maintenant\.endpoint\.(\d+)\.(.+)$`)

// ParsedEndpoint holds a parsed endpoint definition from container labels.
type ParsedEndpoint struct {
	LabelKey     string
	EndpointType EndpointType
	Target       string
	Config       EndpointConfig
	Index        int
}

// LabelParseError represents a label validation error.
type LabelParseError struct {
	LabelKey string
	Value    string
	Message  string
}

func (e *LabelParseError) Error() string {
	return fmt.Sprintf("label %s=%s: %s", e.LabelKey, e.Value, e.Message)
}

// ParseEndpointLabels extracts endpoint definitions from a Docker container's labels.
// Returns parsed endpoints and any configuration errors encountered; an ignored
// container declares none.
func ParseEndpointLabels(labels map[string]string, logger *slog.Logger) ([]*ParsedEndpoint, []*LabelParseError) {
	if container.IgnoredByLabels(labels) {
		return nil, nil
	}
	endpointMap := make(map[int]*ParsedEndpoint)
	globalConfig := make(map[string]string)
	indexedConfigs := make(map[int]map[string]string)
	var parseErrors []*LabelParseError

	for key, value := range labels {
		if !strings.HasPrefix(key, labelPrefix) {
			continue
		}

		suffix := key[len(labelPrefix):]

		// Simple endpoint: maintenant.endpoint.http or maintenant.endpoint.tcp
		if suffix == "http" || suffix == "tcp" {
			ep, err := parseEndpointTarget(key, EndpointType(suffix), value)
			if err != nil {
				parseErrors = append(parseErrors, err)
				logger.Warn("malformed endpoint label", "label", key, "value", value, "error", err.Message)
				continue
			}
			ep.Index = 0
			// Non-indexed takes precedence for index 0
			endpointMap[0] = ep
			continue
		}

		// Indexed endpoint: maintenant.endpoint.N.http or maintenant.endpoint.N.tcp
		if m := indexedLabelRe.FindStringSubmatch(key); m != nil {
			idx, _ := strconv.Atoi(m[1])
			epType := EndpointType(m[2])
			ep, err := parseEndpointTarget(key, epType, value)
			if err != nil {
				parseErrors = append(parseErrors, err)
				logger.Warn("malformed endpoint label", "label", key, "value", value, "error", err.Message)
				continue
			}
			ep.Index = idx
			// Only set if not already set by non-indexed for index 0
			if idx != 0 || endpointMap[0] == nil {
				endpointMap[idx] = ep
			}
			continue
		}

		// Indexed config: maintenant.endpoint.N.something
		if m := indexedConfigRe.FindStringSubmatch(key); m != nil {
			idx, _ := strconv.Atoi(m[1])
			configKey := m[2]
			// Skip if this matched as an endpoint type above
			if configKey == "http" || configKey == "tcp" {
				continue
			}
			if indexedConfigs[idx] == nil {
				indexedConfigs[idx] = make(map[string]string)
			}
			indexedConfigs[idx][configKey] = value
			continue
		}

		// Global config: maintenant.endpoint.interval, maintenant.endpoint.timeout, etc.
		// Also HTTP-specific: maintenant.endpoint.http.method, etc.
		globalConfig[suffix] = value
	}

	// Apply configuration to endpoints
	var endpoints []*ParsedEndpoint
	for idx, ep := range endpointMap {
		cfg := DefaultConfig()

		// Apply global config first
		applyConfigLabels(&cfg, globalConfig, logger)

		// Apply indexed config (overrides global)
		if ic, ok := indexedConfigs[idx]; ok {
			applyConfigLabels(&cfg, ic, logger)
		}

		// Validate timeout <= interval
		if cfg.Timeout > cfg.Interval {
			logger.Warn("endpoint timeout > interval, adjusting", "endpoint", ep.Target, "timeout", cfg.Timeout, "interval", cfg.Interval)
			cfg.Timeout = cfg.Interval
		}

		ep.Config = cfg
		endpoints = append(endpoints, ep)
	}

	return endpoints, parseErrors
}

func parseEndpointTarget(labelKey string, epType EndpointType, value string) (*ParsedEndpoint, *LabelParseError) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, &LabelParseError{LabelKey: labelKey, Value: value, Message: "empty target"}
	}

	if err := ValidateTarget(epType, value); err != nil {
		return nil, &LabelParseError{LabelKey: labelKey, Value: value, Message: err.Error()}
	}

	return &ParsedEndpoint{
		LabelKey:     labelKey,
		EndpointType: epType,
		Target:       value,
	}, nil
}

// ValidateTarget reports why target cannot be probed as an endpoint of type t.
func ValidateTarget(t EndpointType, target string) error {
	switch t {
	case TypeHTTP:
		u, err := url.Parse(target)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return errors.New("invalid HTTP URL: must have http:// or https:// scheme and host")
		}
	case TypeTCP:
		host, port, err := net.SplitHostPort(target)
		if err != nil {
			return errors.New("invalid TCP target: must be host:port format")
		}
		if host == "" {
			return errors.New("invalid TCP target: empty host")
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return errors.New("invalid TCP target: port must be a number between 1 and 65535")
		}
	default:
		return fmt.Errorf("unknown endpoint type %q", t)
	}
	return nil
}

func applyConfigLabels(cfg *EndpointConfig, labels map[string]string, logger *slog.Logger) {
	if v, ok := labels["interval"]; ok {
		d, err := time.ParseDuration(v)
		switch {
		case err != nil || d <= 0:
			logger.Warn("invalid endpoint interval", "value", v)
		case d < MinInterval:
			logger.Warn("endpoint interval below the minimum, raised to it", "value", v, "minimum", MinInterval)
			cfg.Interval = Duration(MinInterval)
		default:
			cfg.Interval = Duration(d)
		}
	}
	if v, ok := labels["timeout"]; ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Timeout = Duration(d)
		} else {
			logger.Warn("invalid endpoint timeout", "value", v)
		}
	}
	if v, ok := labels["failure-threshold"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.FailureThreshold = n
		}
	}
	if v, ok := labels["recovery-threshold"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RecoveryThreshold = n
		}
	}
	if v, ok := labels["http.method"]; ok {
		v = strings.ToUpper(strings.TrimSpace(v))
		switch v {
		case "GET", "HEAD", "POST", "PUT", "DELETE", "PATCH", "OPTIONS":
			cfg.Method = v
		default:
			logger.Warn("invalid HTTP method", "value", v)
		}
	}
	if v, ok := labels["http.expected-status"]; ok {
		cfg.ExpectedStatus = v
	}
	if v, ok := labels["http.tls-verify"]; ok {
		switch strings.ToLower(v) {
		case "false", "0", "no":
			cfg.TLSVerify = false
		case "true", "1", "yes":
			cfg.TLSVerify = true
		}
	}
	if v, ok := labels["http.headers"]; ok {
		headers := parseHeaders(v)
		if headers != nil {
			cfg.Headers = headers
		}
	}
	if v, ok := labels["http.max-redirects"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			cfg.MaxRedirects = n
		}
	}
}

func parseHeaders(v string) map[string]string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}

	// Try JSON first
	if strings.HasPrefix(v, "{") {
		var headers map[string]string
		if err := json.Unmarshal([]byte(v), &headers); err == nil {
			return headers
		}
	}

	// Fallback: key=val,key=val format
	headers := make(map[string]string)
	pairs := strings.Split(v, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		eqIdx := strings.Index(pair, "=")
		if eqIdx > 0 {
			headers[strings.TrimSpace(pair[:eqIdx])] = strings.TrimSpace(pair[eqIdx+1:])
		}
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

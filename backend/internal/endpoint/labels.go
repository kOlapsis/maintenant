package endpoint

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	labelPrefix = "pulseboard.endpoint."
)

// Indexed label regex: pulseboard.endpoint.N.http or pulseboard.endpoint.N.tcp
var indexedLabelRe = regexp.MustCompile(`^pulseboard\.endpoint\.(\d+)\.(http|tcp)$`)

// Indexed config regex: pulseboard.endpoint.N.http.method, pulseboard.endpoint.N.interval, etc.
var indexedConfigRe = regexp.MustCompile(`^pulseboard\.endpoint\.(\d+)\.(.+)$`)

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
// Returns parsed endpoints and any configuration errors encountered.
func ParseEndpointLabels(labels map[string]string, logger *slog.Logger) ([]*ParsedEndpoint, []*LabelParseError) {
	endpointMap := make(map[int]*ParsedEndpoint)
	globalConfig := make(map[string]string)
	indexedConfigs := make(map[int]map[string]string)
	var parseErrors []*LabelParseError

	for key, value := range labels {
		if !strings.HasPrefix(key, labelPrefix) {
			continue
		}

		suffix := key[len(labelPrefix):]

		// Simple endpoint: pulseboard.endpoint.http or pulseboard.endpoint.tcp
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

		// Indexed endpoint: pulseboard.endpoint.N.http or pulseboard.endpoint.N.tcp
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

		// Indexed config: pulseboard.endpoint.N.something
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

		// Global config: pulseboard.endpoint.interval, pulseboard.endpoint.timeout, etc.
		// Also HTTP-specific: pulseboard.endpoint.http.method, etc.
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

		// Validate timeout < interval
		if cfg.Timeout >= cfg.Interval {
			logger.Warn("endpoint timeout >= interval, adjusting", "endpoint", ep.Target, "timeout", cfg.Timeout, "interval", cfg.Interval)
			cfg.Timeout = time.Duration(float64(cfg.Interval) * 0.8)
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

	switch epType {
	case TypeHTTP:
		u, err := url.Parse(value)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, &LabelParseError{LabelKey: labelKey, Value: value, Message: "invalid HTTP URL: must have http:// or https:// scheme and host"}
		}
	case TypeTCP:
		// Expect host:port format
		if !strings.Contains(value, ":") {
			return nil, &LabelParseError{LabelKey: labelKey, Value: value, Message: "invalid TCP target: must be host:port format"}
		}
		parts := strings.SplitN(value, ":", 2)
		if parts[0] == "" {
			return nil, &LabelParseError{LabelKey: labelKey, Value: value, Message: "invalid TCP target: empty host"}
		}
		if _, err := strconv.Atoi(parts[1]); err != nil {
			return nil, &LabelParseError{LabelKey: labelKey, Value: value, Message: "invalid TCP target: port must be numeric"}
		}
	}

	return &ParsedEndpoint{
		LabelKey:     labelKey,
		EndpointType: epType,
		Target:       value,
	}, nil
}

func applyConfigLabels(cfg *EndpointConfig, labels map[string]string, logger *slog.Logger) {
	if v, ok := labels["interval"]; ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Interval = d
		} else {
			logger.Warn("invalid endpoint interval", "value", v)
		}
	}
	if v, ok := labels["timeout"]; ok {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Timeout = d
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

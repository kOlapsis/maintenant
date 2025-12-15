package endpoint

import (
	"strconv"
	"strings"
)

// StatusMatcher evaluates whether an HTTP status code matches the expected pattern.
type StatusMatcher struct {
	ranges []statusRange
}

type statusRange struct {
	min int
	max int
}

// NewStatusMatcher parses an expected-status string (e.g., "2xx", "200,201", "2xx,301")
// and returns a StatusMatcher. Returns a default 2xx matcher on empty input.
func NewStatusMatcher(pattern string) *StatusMatcher {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		pattern = "2xx"
	}

	var ranges []statusRange
	parts := strings.Split(pattern, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		r, ok := parseStatusPart(part)
		if ok {
			ranges = append(ranges, r)
		}
	}

	if len(ranges) == 0 {
		// Fallback to 2xx
		ranges = []statusRange{{min: 200, max: 299}}
	}

	return &StatusMatcher{ranges: ranges}
}

func parseStatusPart(s string) (statusRange, bool) {
	s = strings.ToLower(s)

	// Range shorthand: "2xx", "3xx", "4xx", "5xx"
	if len(s) == 3 && s[1] == 'x' && s[2] == 'x' {
		digit := int(s[0] - '0')
		if digit >= 1 && digit <= 5 {
			return statusRange{min: digit * 100, max: digit*100 + 99}, true
		}
	}

	// Exact code: "200", "301", etc.
	code, err := strconv.Atoi(s)
	if err == nil && code >= 100 && code <= 599 {
		return statusRange{min: code, max: code}, true
	}

	return statusRange{}, false
}

// Matches returns true if the given HTTP status code matches any of the expected patterns.
func (m *StatusMatcher) Matches(statusCode int) bool {
	for _, r := range m.ranges {
		if statusCode >= r.min && statusCode <= r.max {
			return true
		}
	}
	return false
}

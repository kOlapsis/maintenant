package oauth

import (
	"errors"
	"fmt"
	"strings"
)

// ParseScope normalises an OAuth scope request: empty or "read" yields "read", a set of read and write yields "read write", anything else is an error.
func ParseScope(raw string) (string, error) {
	var read, write bool
	for _, s := range strings.Fields(raw) {
		switch s {
		case "read":
			read = true
		case "write":
			write = true
		default:
			return "", fmt.Errorf("unknown scope %q: this server grants read or read write", s)
		}
	}
	switch {
	case write && !read:
		return "", errors.New("scope write needs read: ask for read write")
	case write:
		return "read write", nil
	default:
		return "read", nil
	}
}

func grantedScope(stored string) string {
	scope, err := ParseScope(stored)
	if err != nil {
		return "read"
	}
	return scope
}

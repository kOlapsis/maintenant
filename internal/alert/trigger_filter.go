// Copyright 2026 Benjamin Touchard (Kolapsis)
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"fmt"
	"strings"
)

// ValidateSeverityFilter checks that every item of a comma-separated trigger severity filter is a known severity.
func ValidateSeverityFilter(csv string) error {
	for _, item := range strings.Split(csv, ",") {
		if item = strings.TrimSpace(item); item != "" && !IsSeverity(item) {
			return fmt.Errorf("field=filter_severities: %q is not a severity (info, warning, critical)", item)
		}
	}
	return nil
}

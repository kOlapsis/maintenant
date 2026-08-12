// Copyright 2026 Benjamin Touchard (Kolapsis)
//
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0)
// or a commercial license. You may not use this file except in compliance
// with one of these licenses.
//
// AGPL-3.0: https://www.gnu.org/licenses/agpl-3.0.html
// Commercial: See COMMERCIAL-LICENSE.md
//
// Source: https://github.com/kolapsis/maintenant

package extension

import "errors"

// Edition identifies the capability tier the running binary operates at.
// The three editions are ordered: Community < Personal < Pro.
type Edition string

const (
	Community Edition = "community"
	Personal  Edition = "personal"
	Pro       Edition = "pro"
)

// rank orders the editions. An unrecognised edition ranks -1 and never
// satisfies AtLeast, so a value this binary does not know grants nothing.
func (e Edition) rank() int {
	switch e {
	case Community:
		return 0
	case Personal:
		return 1
	case Pro:
		return 2
	default:
		return -1
	}
}

// AtLeast reports whether e is at or above other in the edition order.
// It is false as soon as either side is unrecognised.
func (e Edition) AtLeast(other Edition) bool {
	er, or := e.rank(), other.rank()
	if er < 0 || or < 0 {
		return false
	}
	return er >= or
}

// ParseEdition converts a wire value to an Edition. The second result reports
// whether the value was recognised; callers decide what an unknown value means.
func ParseEdition(s string) (Edition, bool) {
	e := Edition(s)
	if e.rank() < 0 {
		return Community, false
	}
	return e, true
}

// ErrNotAvailable is returned by no-op implementations when an extension is not available.
var ErrNotAvailable = errors.New("this feature requires an extended edition of maintenant")

// CurrentEdition returns the edition of the running binary.
// CE always returns Community. Extended editions override this via the build.
var CurrentEdition = func() Edition { return Community }

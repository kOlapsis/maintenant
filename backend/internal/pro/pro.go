package pro

import "errors"

// Edition identifies whether the running binary is Community or Pro.
type Edition string

const (
	Community Edition = "community"
	Pro       Edition = "pro"
)

// ErrProFeature is returned by no-op implementations when a Pro feature is invoked on CE.
var ErrProFeature = errors.New("this feature requires pulseboard Pro")

// CurrentEdition returns the edition of the running binary.
// CE always returns Community. The Pro repo overrides this via the build.
var CurrentEdition = func() Edition { return Community }

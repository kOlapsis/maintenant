package extension

import "errors"

// Edition identifies whether the running binary is Community or Enterprise.
type Edition string

const (
	Community  Edition = "community"
	Enterprise Edition = "enterprise"
)

// ErrNotAvailable is returned by no-op implementations when an extension is not available.
var ErrNotAvailable = errors.New("this feature requires an extended edition of maintenant")

// CurrentEdition returns the edition of the running binary.
// CE always returns Community. Extended editions override this via the build.
var CurrentEdition = func() Edition { return Community }

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

// PlanTier identifies the subscription tier of the current Pro installation.
type PlanTier string

const (
	PlanTierNone     PlanTier = "none"
	PlanTierSolo     PlanTier = "solo"
	PlanTierTeam     PlanTier = "team"
	PlanTierBusiness PlanTier = "business"
)

// CurrentPlanTier returns the current plan tier.
// CE always returns PlanTierNone. Pro overrides this via build.
var CurrentPlanTier = func() PlanTier { return PlanTierNone }

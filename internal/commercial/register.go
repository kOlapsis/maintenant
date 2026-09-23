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
package commercial

import (
	"github.com/kolapsis/maintenant/internal/commercial/license"
	"github.com/kolapsis/maintenant/internal/commercial/tiers"
	"github.com/kolapsis/maintenant/internal/extension"
)

// Register installs the commercial tier table and the licence-backed edition source into the core.
func Register() {
	extension.Register(tiers.Policy{}, newEditionSource)
}

func newEditionSource(cfg extension.SourceConfig) (extension.EditionSource, error) {
	if cfg.LicenseKey == "" {
		return nil, nil
	}
	license.InitPublicKey(cfg.PublicKeyB64)
	m, err := license.NewManager(cfg.LicenseKey, cfg.DataDir, cfg.Version, cfg.BuildDate, cfg.Logger)
	if err != nil {
		return nil, err
	}
	return m, nil
}

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
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const (
	modulePath     = "github.com/kolapsis/maintenant"
	commercialPath = modulePath + "/internal/commercial"
)

func TestNoCorePackageImportsCommercial(t *testing.T) {
	gomod, err := exec.Command("go", "env", "GOMOD").Output()
	if err != nil {
		t.Fatalf("go env GOMOD: %v", err)
	}

	cmd := exec.Command("go", "list", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", "./...")
	cmd.Dir = filepath.Dir(strings.TrimSpace(string(gomod)))
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		pkg := fields[0]
		if isCommercial(pkg) || pkg == modulePath+"/cmd/maintenant" {
			continue
		}
		for _, imp := range fields[1:] {
			if isCommercial(imp) {
				t.Errorf("%s imports %s: only cmd/maintenant may import internal/commercial", pkg, imp)
			}
		}
	}
}

func isCommercial(pkg string) bool {
	return pkg == commercialPath || strings.HasPrefix(pkg, commercialPath+"/")
}

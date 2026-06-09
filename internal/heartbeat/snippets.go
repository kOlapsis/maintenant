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

package heartbeat

import "fmt"

// GenerateSnippets returns integration code snippets for a heartbeat monitor.
// token is the heartbeat's ping token (its id).
func GenerateSnippets(baseURL, token string) map[string]string {
	pingURL := baseURL + "/ping/" + token
	startURL := pingURL + "/start"

	return map[string]string{
		"curl": fmt.Sprintf(`# Simple ping
curl -fsS -m 10 --retry 5 %s

# With start/finish tracking
curl -fsS -m 10 --retry 5 %s
# ... your job here ...
curl -fsS -m 10 --retry 5 %s`, pingURL, startURL, pingURL),

		"wget": fmt.Sprintf(`wget -q --spider %s`, pingURL),

		"python": fmt.Sprintf(`import urllib.request
urllib.request.urlopen('%s')`, pingURL),

		"go": fmt.Sprintf(`http.Get("%s")`, pingURL),

		"bash": fmt.Sprintf(`#!/bin/bash
set -e

# Signal start
curl -fsS -m 10 --retry 5 %s

# Your job here
/path/to/your/job.sh

# Signal success
curl -fsS -m 10 --retry 5 %s`, startURL, pingURL),

		"docker_healthcheck": fmt.Sprintf(`HEALTHCHECK CMD curl -f %s || exit 1`, pingURL),
	}
}
